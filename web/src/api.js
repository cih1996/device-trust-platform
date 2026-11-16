import { ref } from 'vue'

const KEY = 'dtp_admin_session'
function stored() {
  try { return localStorage.getItem(KEY) || '' } catch { return '' }
}
export const token = ref(stored())
function storedUser() {
  try { return localStorage.getItem(KEY + '_user') || 'admin' } catch { return 'admin' }
}
export const username = ref(storedUser())
export function setUser(value) {
  username.value = value
  try { localStorage.setItem(KEY + '_user', value) } catch {}
}

export function setToken(value) {
  token.value = value
  try { value ? localStorage.setItem(KEY, value) : localStorage.removeItem(KEY) } catch {}
}

export class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status }
}

export async function api(path, method = 'GET', body) {
  const response = await fetch('/api/' + path, {
    method,
    headers: { Authorization: 'Bearer ' + token.value, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await response.json().catch(() => ({ error: '服务返回了无法解析的内容（' + response.status + '）' }))
  if (response.status === 401 && path !== 'auth/login') setToken('')
  if (!response.ok) throw new ApiError(data.error || '请求失败（' + response.status + '）', response.status)
  return data
}

export function query(params) {
  const search = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== '' && v != null) search.set(k, v)
  const text = search.toString()
  return text ? '?' + text : ''
}
