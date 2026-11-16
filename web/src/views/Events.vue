<script setup>
import { ref, computed, watch } from 'vue'
import { api, query } from '../api.js'
import { route, setQuery, go } from '../router.js'
import { appId, appName, currentApp, revision, report, openEvent } from '../store.js'
import { dateTime, fullTime, shortId } from '../format.js'
import { sceneNames, statusNames, reasonName, reviewNames } from '../risk-display.js'
import RiskScore from '../components/RiskScore.vue'

const LIMIT = 50
const listing = ref(null)
const loading = ref(false)
const search = ref(route.query.q || '')
const range = computed(() => route.query.range || '7d')
const offset = computed(() => Number(route.query.offset) || 0)
const ranges = [['24h', '24 小时'], ['7d', '7 天'], ['30d', '30 天'], ['all', '全部']]
const levels = [['', '全部'], ['high', '高风险'], ['medium', '中风险'], ['low', '低风险']]

async function load() {
  loading.value = true
  try {
    listing.value = await api('events' + query({ appId: appId.value, range: range.value, scene: route.query.scene, level: route.query.level, status: route.query.status, q: route.query.q, profileId: route.query.profileId, offset: offset.value, limit: LIMIT }))
  } catch (e) {
    report(e)
  } finally {
    loading.value = false
  }
}
watch([appId, () => JSON.stringify(route.query), revision], load, { immediate: true })

const set = patch => setQuery({ ...patch, offset: '' })
function submit() { set({ q: search.value.trim() }) }
function pageBy(d) { setQuery({ offset: Math.max(0, offset.value + d * LIMIT) || '' }) }
const counts = computed(() => listing.value?.counts || {})
</script>

<template>
  <div>
    <div class="page-head">
      <div><h1>风险事件</h1><p>{{ currentApp ? currentApp.name : '全部应用' }} · 业务服务提交的注册、营销、支付等事件</p></div>
      <div class="seg" aria-label="时间范围">
        <button v-for="[key, label] in ranges" :key="key" :class="{ on: range === key }" @click="set({ range: key })">{{ label }}</button>
      </div>
    </div>

    <section class="card" :class="{ loading }">
      <div class="filters">
        <form class="search" @submit.prevent="submit" style="display:flex;gap:8px">
          <input v-model="search" type="search" placeholder="账号、事件 ID、设备 ID 或 IP" aria-label="搜索事件" style="flex:1">
          <button>搜索</button>
        </form>
        <div class="seg" aria-label="风险等级">
          <button v-for="[key, label] in levels" :key="key" :class="{ on: (route.query.level || '') === key }" @click="set({ level: key })">{{ label }}<span v-if="key" class="n">{{ counts[key] ?? '' }}</span></button>
        </div>
        <select :value="route.query.scene || ''" @change="set({ scene: $event.target.value })" aria-label="业务场景">
          <option value="">全部场景</option>
          <option v-for="(label, key) in sceneNames" :key="key" :value="key">{{ label }}</option>
        </select>
        <select :value="route.query.status || ''" @change="set({ status: $event.target.value })" aria-label="处置状态">
          <option value="">全部状态</option>
          <option v-for="(label, key) in statusNames" :key="key" :value="key">{{ label }}{{ key === 'pending' && counts.pending ? ' (' + counts.pending + ')' : '' }}</option>
        </select>
        <span v-if="route.query.profileId" class="tag accent">仅看此设备 <a href="#" @click.prevent="set({ profileId: '' })" aria-label="取消设备筛选">×</a></span>
      </div>
      <div class="table-wrap" v-if="listing">
        <table>
          <thead><tr><th>时间</th><th>账号 / 场景</th><th v-if="!appId">应用</th><th>设备</th><th>IP</th><th>风险评分</th><th>命中线索</th><th>页面停留</th><th>处置</th></tr></thead>
          <tbody>
            <tr v-for="e in listing.items" :key="e.id" class="click" @click="openEvent(e.id)">
              <td :title="fullTime(e.receivedAt)">{{ dateTime(e.receivedAt) }}</td>
              <td>{{ e.accountId }}<span class="sub">{{ sceneNames[e.scene] || e.type }} · {{ e.type }}</span></td>
              <td v-if="!appId">{{ appName(e.appId) }}</td>
              <td><a class="mono" :href="'#/devices/' + e.profileId + (appId ? '?app=' + appId : '')" @click.stop>{{ shortId(e.deviceId || e.installationId) }}</a></td>
              <td class="mono">{{ e.clientIp || '—' }}<span v-if="e.resources?.ip" class="sub">{{ e.resources.ip.province || e.resources.ip.country }}</span></td>
              <td><RiskScore :score="e.score" /></td>
              <td><span class="row-gap"><span v-for="h in e.hits.slice(0, 2)" :key="h.code" class="tag">{{ reasonName(h.code) }}</span><span v-if="e.hits.length > 2" class="muted">+{{ e.hits.length - 2 }}</span><span v-if="!e.hits.length" class="muted">—</span></span></td>
              <td class="num">{{ e.context?.pageDurationMs > 0 ? Math.round(e.context.pageDurationMs / 1000) + ' 秒' : '—' }}</td>
              <td><span :class="['tag', { warn: e.status === 'pending', bad: e.status === 'rejected' || e.status === 'failed' }]">{{ statusNames[e.status] }}</span><span v-if="e.feedback" class="sub">{{ reviewNames[e.feedback.label] }}</span></td>
            </tr>
          </tbody>
        </table>
        <div v-if="!listing.items.length" class="empty"><strong>没有符合条件的事件</strong>调整时间范围或筛选条件试试</div>
      </div>
      <div class="pager" v-if="listing && listing.total > LIMIT">
        <span>{{ offset + 1 }}–{{ Math.min(offset + LIMIT, listing.total) }} / 共 {{ listing.total }} 条</span>
        <button class="small" :disabled="offset === 0" @click="pageBy(-1)">上一页</button>
        <button class="small" :disabled="offset + LIMIT >= listing.total" @click="pageBy(1)">下一页</button>
      </div>
    </section>
  </div>
</template>
