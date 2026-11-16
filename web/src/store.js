import { ref, computed, watch } from 'vue'
import { api } from './api.js'
import { route } from './router.js'

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
}

export function report(e) {
  error.value = e?.message || String(e)
}

export function changed() {
  revision.value++
}
export const openEvent = id => { openEventId.value = id }
export const closeEvent = () => { openEventId.value = '' }
export const clearError = () => { error.value = '' }
