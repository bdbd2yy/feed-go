import { beforeEach, describe, expect, it, vi } from 'vitest'
const { auth } = vi.hoisted(() => ({
  auth: { token: 'expired', refreshToken: 'refresh', setToken: vi.fn(), clearTokens: vi.fn() },
}))
vi.mock('../lib/auth', () => ({ auth }))
vi.mock('../utils/error-reporter', () => ({ reportError: vi.fn() }))
import { ApiError, postForm, postJson } from '../api/client'
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
beforeEach(() => {
  vi.clearAllMocks()
  auth.token = 'expired'
  auth.refreshToken = 'refresh'
  auth.setToken.mockImplementation((token: string) => {
    auth.token = token
  })
})
describe('API authentication', () => {
  it('shares a refresh between concurrent unauthorized requests and retries both', async () => {
    let release!: (value: Response) => void
    const refresh = new Promise<Response>((resolve) => {
      release = resolve
    })
    const fetch = vi.fn(async (url: string, options: RequestInit) => {
      if (url.endsWith('/account/refresh')) return refresh
      return response(
        { ok: true },
        (options.headers as Record<string, string>).Authorization === 'Bearer fresh' ? 200 : 401,
      )
    })
    vi.stubGlobal('fetch', fetch)
    const first = postJson('/one', {})
    const second = postForm('/two', new FormData())
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(3))
    release(response({ token: 'fresh' }))
    expect(await Promise.all([first, second])).toEqual([{ ok: true }, { ok: true }])
    expect(fetch.mock.calls.filter(([url]) => url.endsWith('/account/refresh'))).toHaveLength(1)
    expect(auth.setToken).toHaveBeenCalledWith('fresh')
  })
  it('clears the session when refresh fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => response({ error: '登录已过期' }, 401)),
    )
    await expect(postJson('/one', {})).rejects.toBeInstanceOf(ApiError)
    expect(auth.clearTokens).toHaveBeenCalled()
  })
  it('rejects protected requests before sending when no token exists', async () => {
    auth.token = ''
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    await expect(postJson('/one', {}, { authRequired: true })).rejects.toMatchObject({
      status: 401,
    })
    expect(fetch).not.toHaveBeenCalled()
  })
})

describe('session changes during requests', () => {
  it('does not restore a logged-out session when a refresh completes late', async () => {
    let release!: (res: Response) => void
    const pending = new Promise<Response>((resolve) => {
      release = resolve
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        url.endsWith('/account/refresh') ? pending : response({}, 401),
      ),
    )
    const request = postJson('/private', {})
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
    auth.token = ''
    auth.refreshToken = ''
    release(response({ token: 'late' }))
    await expect(request).rejects.toMatchObject({ status: 401 })
    expect(auth.setToken).not.toHaveBeenCalled()
  })
  it('does not clear a new account when an older request returns unauthorized', async () => {
    let release!: (res: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            release = resolve
          }),
      ),
    )
    const request = postJson('/private', {})
    auth.token = 'new-account'
    auth.refreshToken = 'new-refresh'
    release(response({}, 401))
    await expect(request).rejects.toMatchObject({ status: 401 })
    expect(auth.clearTokens).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('reuses the refreshed token for a delayed unauthorized response', async () => {
    let release!: (res: Response) => void
    const late = new Promise<Response>((resolve) => {
      release = resolve
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, options: RequestInit) => {
        if (url.endsWith('/account/refresh')) return response({ token: 'fresh' })
        if ((options.headers as Record<string, string>).Authorization === 'Bearer fresh')
          return response({ ok: true })
        if (url.endsWith('/late')) return late
        return response({}, 401)
      }),
    )
    const delayed = postJson('/late', {})
    await postJson('/first', {})
    release(response({}, 401))
    await expect(delayed).resolves.toEqual({ ok: true })
    expect(
      vi.mocked(fetch).mock.calls.filter(([url]) => String(url).endsWith('/account/refresh')),
    ).toHaveLength(1)
  })
})
