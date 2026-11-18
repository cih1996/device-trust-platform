<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { token, setToken, username } from './api.js'
import { route, go, href, setQuery } from './router.js'
import { apps, appId, error, loadApps, loadMe, me, can, roleNames, openEventId, report, changed, closeEvent, clearError } from './store.js'
import Icon from './components/Icon.vue'
import Login from './views/Login.vue'
import Overview from './views/Overview.vue'
import Devices from './views/Devices.vue'
import DeviceDetail from './views/DeviceDetail.vue'
import Events from './views/Events.vue'
import Graph from './views/Graph.vue'
import Insights from './views/Insights.vue'
import Team from './views/Team.vue'
import Platform from './views/Platform.vue'
import Apps from './views/Apps.vue'
import AppDetail from './views/AppDetail.vue'
import EventDrawer from './views/EventDrawer.vue'
import AccountDialog from './views/AccountDialog.vue'

const ready = ref(false)
const accountOpen = ref(false)
const signedIn = computed(() => !!token.value && ready.value)
const baseNav = [
  { name: 'overview', label: '风险总览', icon: 'overview' },
  { name: 'insights', label: '大盘分析', icon: 'insights' },
  { name: 'devices', label: '设备', icon: 'devices' },
  { name: 'graph', label: '关系图谱', icon: 'graph' },
  { name: 'events', label: '风险事件', icon: 'events' },
  { name: 'apps', label: '应用管理', icon: 'apps' },
]
const nav = computed(() => [
  ...baseNav,
  ...(me.value?.team?.id && can('admin') ? [{ name: 'team', label: '团队成员', icon: 'team' }] : []),
  ...(can('platform') ? [{ name: 'platform', label: '平台管理', icon: 'platform' }] : []),
])
const view = computed(() => {
  // Pages this account may not open (a link or the page left by another account) fall back to the overview.
  if (route.name === 'team' && !(me.value?.team?.id && can('admin'))) return Overview
  if (route.name === 'platform' && !can('platform')) return Overview
  if (route.name === 'devices') return route.id ? DeviceDetail : Devices
  if (route.name === 'apps') return route.id ? AppDetail : Apps
  return { overview: Overview, events: Events, graph: Graph, insights: Insights, team: Team, platform: Platform }[route.name] || Overview
})
const scoped = computed(() => !['apps', 'team', 'platform'].includes(route.name))

async function start() {
  if (!token.value) return
  try {
    await Promise.all([loadApps(), loadMe()])
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
  me.value = null
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
        <div class="who">{{ me?.username || username }}<small v-if="me">{{ me.team?.name ? me.team.name + ' · ' : '' }}{{ roleNames[me.role] }}</small></div>
        <a class="nav-item" href="https://devicetrust.lingzidev.com/docs/" target="_blank" rel="noopener"><Icon name="docs" /><span class="label">接入文档</span></a>
        <a v-if="!me?.demo" class="nav-item" href="#" @click.prevent="accountOpen = true"><Icon name="account" /><span class="label">账号设置</span></a>
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
        <strong v-else>{{ { apps: '应用管理', team: '团队成员', platform: '平台管理' }[route.name] }}</strong>
        <span class="spacer"></span>
        <button class="ghost" @click="changed(); loadApps().catch(report)" title="刷新数据"><Icon name="refresh" />刷新</button>
      </header>
      <div class="content">
        <div v-if="me?.demo" class="notice">公开体验账号：可查看示例 App 的设备与事件，并处理待核验事件；所有体验者共用此账号，数据互相可见。</div>
        <div v-if="error" class="banner" role="alert"><span>{{ error }}</span><button class="ghost small" @click="clearError">关闭</button></div>
        <section v-if="!apps.length && scoped" class="card onboarding">
          <h1>{{ can('admin') ? '欢迎使用知衡，先接入第一个应用' : '团队还没有接入应用' }}</h1>
          <template v-if="can('admin')">
            <ol>
              <li><strong>创建应用</strong>：填写 App 名称和 Android 包名，保存应用 ID 和应用密钥。<a :href="href('apps')">去创建 →</a></li>
              <li><strong>集成 SDK</strong>：App 集成 Android SDK，业务后端调用事件接口。<a href="https://devicetrust.lingzidev.com/docs/" target="_blank" rel="noopener">查看接入文档 →</a></li>
              <li><strong>查看风险</strong>：设备开始上报后，风险总览、大盘分析和设备页会出现数据。</li>
              <li v-if="me?.team?.id"><strong>邀请同事</strong>（可选）：在“团队成员”生成邀请链接。<a :href="href('team')">去邀请 →</a></li>
            </ol>
          </template>
          <p v-else class="muted">请联系团队管理员在“应用管理”中创建应用。创建后这里会显示数据。</p>
        </section>
        <component :is="view" v-else :key="route.name + (route.id ? '/' + route.id : '')" />
      </div>
    </div>
    <EventDrawer v-if="openEventId" :id="openEventId" @close="closeEvent" />
    <AccountDialog v-if="accountOpen" @close="accountOpen = false" />
  </div>
</template>
