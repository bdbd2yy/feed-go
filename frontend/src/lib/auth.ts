import { useSyncExternalStore } from 'react'
import { decodeJwtPayload } from '../utils/jwt'

function read(key: string) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
let snapshot = { token: read('access_token'), refreshToken: read('refresh_token') }
const listeners = new Set<() => void>()
function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
function update(token: string | null, refreshToken: string | null) {
  const previousAccount = snapshot.token ? decodeJwtPayload(snapshot.token)?.account_id : null
  const nextAccount = token ? decodeJwtPayload(token)?.account_id : null
  const changed = previousAccount !== nextAccount || !!snapshot.token !== !!token
  snapshot = { token, refreshToken }
  for (const [key, value] of [
    ['access_token', token],
    ['refresh_token', refreshToken],
  ]) {
    try {
      if (value) localStorage.setItem(key!, value)
      else localStorage.removeItem(key!)
    } catch {
      /* Storage may be unavailable. */
    }
  }
  if (changed) window.dispatchEvent(new Event('sessionchange'))
  listeners.forEach((listener) => listener())
}
export const auth = {
  get token() {
    return snapshot.token
  },
  get refreshToken() {
    return snapshot.refreshToken
  },
  setToken(token: string) {
    update(token, snapshot.refreshToken)
  },
  setTokens(token: string, refreshToken: string) {
    update(token, refreshToken)
  },
  clearTokens() {
    update(null, null)
  },
}
export function useAuth() {
  const state = useSyncExternalStore(subscribe, () => snapshot)
  return {
    ...state,
    isLoggedIn: !!state.token,
    claims: state.token ? decodeJwtPayload(state.token) : null,
  }
}
window.addEventListener('storage', (event) => {
  if (event.key === null || event.key === 'access_token' || event.key === 'refresh_token') {
    snapshot = { token: read('access_token'), refreshToken: read('refresh_token') }
    window.dispatchEvent(new Event('sessionchange'))
    listeners.forEach((listener) => listener())
  }
})
