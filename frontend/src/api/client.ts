import { auth } from '../lib/auth'
import { reportError } from '../utils/error-reporter'

export class ApiError extends Error {
  status: number
  payload?: unknown

  constructor(message: string, status: number, payload?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.payload = payload
  }
}

type ApiErrorBody = { error?: string }

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api'

let refreshTask: { credential: string; promise: Promise<string | null> } | null = null

async function tryRefresh(
  token: string | null,
  refreshToken: string | null,
): Promise<string | null> {
  if (!refreshToken || auth.refreshToken !== refreshToken) return null
  if (auth.token !== token) return auth.token
  if (refreshTask?.credential === refreshToken) return refreshTask.promise
  const promise = (async () => {
    try {
      const res = await fetch(`${API_BASE}/account/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })
      if (auth.refreshToken !== refreshToken || auth.token !== token) return null
      if (!res.ok) {
        auth.clearTokens()
        return null
      }
      const data = await res.json()
      if (auth.refreshToken !== refreshToken || auth.token !== token) return null
      if (typeof data.token !== 'string' || !data.token) throw new Error('刷新响应缺少 token')
      auth.setToken(data.token)
      return data.token as string
    } catch {
      if (auth.refreshToken === refreshToken && auth.token === token) auth.clearTokens()
      return null
    }
  })()
  refreshTask = { credential: refreshToken, promise }
  try {
    return await promise
  } finally {
    if (refreshTask?.promise === promise) refreshTask = null
  }
}

async function request<T>(
  path: string,
  body: BodyInit,
  headers: Record<string, string>,
  authRequired?: boolean,
): Promise<T> {
  let token = auth.token
  const refreshToken = auth.refreshToken
  if (authRequired && !token) throw new ApiError('需要先登录（缺少 token）', 401)
  if (token) headers.Authorization = `Bearer ${token}`
  let res = await fetch(`${API_BASE}${path}`, { method: 'POST', headers, body })
  if (res.status === 401 && token && path !== '/account/refresh' && path !== '/account/login') {
    const refreshed = await tryRefresh(token, refreshToken)
    if (refreshed) {
      token = refreshed
      headers.Authorization = `Bearer ${refreshed}`
      res = await fetch(`${API_BASE}${path}`, { method: 'POST', headers, body })
    }
  }
  return handleResponse<T>(res, path, token)
}

export function postJson<T>(
  path: string,
  body: unknown,
  options?: { authRequired?: boolean },
): Promise<T> {
  return request(
    path,
    JSON.stringify(body ?? {}),
    { 'Content-Type': 'application/json' },
    options?.authRequired,
  )
}

export function postForm<T>(
  path: string,
  body: FormData,
  options?: { authRequired?: boolean },
): Promise<T> {
  return request(path, body, {}, options?.authRequired)
}

async function handleResponse<T>(res: Response, path: string, token: string | null): Promise<T> {
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!res.ok) {
    if (res.status === 401 && token && auth.token === token) auth.clearTokens()
    const msg =
      data && typeof data === 'object' && (data as ApiErrorBody).error
        ? String((data as ApiErrorBody).error)
        : `请求失败 (${res.status})`
    const apiErr = new ApiError(msg, res.status, data)
    reportError(apiErr, { path, status: res.status })
    throw apiErr
  }

  return data as T
}
