import { ref, computed, watch } from 'vue'
import { api } from './api.js'
import { route, setQuery } from './router.js'

const APP_KEY = 'dtp_selected_app'
function remembered() {
  try { return localStorage.getItem(APP_KEY) || '' } catch { return '' }
}

export const apps = ref([])
export const error = ref('')
export const openEventId = ref('')
export const revision = ref(0) // bumps after mutations so views reload

// The working app comes from the URL, falling back to the last one chosen.
export const appId = computed(() => (route.query.app ?? remembered()))
watch(appId, id => { try { localStorage.setItem(APP_KEY, id) } catch {} }, { immediate: true })

export const currentApp = computed(() => apps.value.find(a => a.id === appId.value) || null)
export const appName = id => apps.value.find(a => a.id === id)?.name || id

export async function loadApps() {
  apps.value = await api('apps')
  // A linked or remembered app this account cannot see (chosen under another
  // account in this browser) falls back to all apps.
  if (appId.value && !apps.value.some(a => a.id === appId.value)) {
    try { localStorage.setItem(APP_KEY, '') } catch {}
    setQuery({ app: '' })
  }
}

// The signed-in account: role is platform | admin | analyst | viewer.
export const me = ref(null)
export async function loadMe() {
  me.value = await api('auth/me')
}
const rank = { viewer: 1, analyst: 2, admin: 3, platform: 4 }
export const can = role => rank[me.value?.role] >= rank[role]
export const roleNames = { platform: '平台管理员', admin: '管理员', analyst: '运营', viewer: '只读' }

export function report(e) {
  error.value = e?.message || String(e)
}

export function changed() {
  revision.value++
}
export const openEvent = id => { openEventId.value = id }
export const closeEvent = () => { openEventId.value = '' }
export const clearError = () => { error.value = '' }
