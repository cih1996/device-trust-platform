<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { token, setToken, username } from './api.js'
import { route, go, href, setQuery } from './router.js'
import { apps, appId, error, loadApps, openEventId, report, changed, closeEvent, clearError } from './store.js'
import Icon from './components/Icon.vue'
import Login from './views/Login.vue'
import Overview from './views/Overview.vue'
import Devices from './views/Devices.vue'
import DeviceDetail from './views/DeviceDetail.vue'
import Events from './views/Events.vue'
import Apps from './views/Apps.vue'
import AppDetail from './views/AppDetail.vue'
import EventDrawer from './views/EventDrawer.vue'
import AccountDialog from './views/AccountDialog.vue'

const ready = ref(false)
const accountOpen = ref(false)
const signedIn = computed(() => !!token.value && ready.value)
const nav = [
  { name: 'overview', label: '风险总览', icon: 'overview' },
  { name: 'devices', label: '设备', icon: 'devices' },
  { name: 'events', label: '风险事件', icon: 'events' },
  { name: 'apps', label: '应用管理', icon: 'apps' },
]
const view = computed(() => {
  if (route.name === 'devices') return route.id ? DeviceDetail : Devices
  if (route.name === 'apps') return route.id ? AppDetail : Apps
  return { overview: Overview, events: Events }[route.name] || Overview
})
const scoped = computed(() => route.name !== 'apps')

async function start() {
  if (!token.value) return
  try {
    await loadApps()
    ready.value = true
    if (appId.value && !apps.value.some(a => a.id === appId.value)) setQuery({ app: '' })
  } catch (e) {
    if (token.value) report(e)
  }
}
async function signedInNow() {
  await start()
}
function logout() {
  setToken('')
  ready.value = false
  apps.value = []
}
function pickApp(event) {
  setQuery({ app: event.target.value })
}
watch(token, value => { if (!value) ready.value = false })
// Navigating anywhere closes the event drawer so it never covers the next page.
watch(() => [route.name, route.id], closeEvent)
onMounted(start)
</script>

<template>
  <Login v-if="!signedIn" @done="signedInNow" />
  <div v-else class="shell">
    <aside class="sidebar">
      <a class="brand" :href="href('overview', '', { app: appId })"><span class="mark">衡</span>知衡</a>
      <a v-for="item in nav" :key="item.name" :href="href(item.name, '', { app: appId })" :class="['nav-item', { active: route.name === item.name }]">
        <Icon :name="item.icon" /><span class="label">{{ item.label }}</span>
      </a>
      <div class="sidebar-foot">
        <div class="who">{{ username }}</div>
        <a class="nav-item" href="https://devicetrust.lingzidev.com/docs/" target="_blank" rel="noopener"><Icon name="docs" /><span class="label">接入文档</span></a>
        <a class="nav-item" href="#" @click.prevent="accountOpen = true"><Icon name="account" /><span class="label">账号设置</span></a>
        <a class="nav-item" href="#" @click.prevent="logout"><Icon name="logout" /><span class="label">退出登录</span></a>
      </div>
    </aside>
    <div class="main">
      <header class="topbar">
        <label v-if="scoped" class="app-switch">
          <span class="muted">应用</span>
          <select :value="appId" @change="pickApp" aria-label="切换应用">
            <option value="">全部应用</option>
            <option v-for="a in apps" :key="a.id" :value="a.id">{{ a.name }}</option>
          </select>
        </label>
        <strong v-else>应用管理</strong>
        <span class="spacer"></span>
        <button class="ghost" @click="changed(); loadApps().catch(report)" title="刷新数据"><Icon name="refresh" />刷新</button>
      </header>
      <div class="content">
        <div v-if="error" class="banner" role="alert"><span>{{ error }}</span><button class="ghost small" @click="clearError">关闭</button></div>
        <component :is="view" :key="route.name + (route.id ? '/' + route.id : '')" />
      </div>
    </div>
    <EventDrawer v-if="openEventId" :id="openEventId" @close="closeEvent" />
    <AccountDialog v-if="accountOpen" @close="accountOpen = false" />
  </div>
</template>
