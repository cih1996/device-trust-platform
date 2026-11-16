<script setup>
import { ref, computed, watch } from 'vue'
import { api, query } from '../api.js'
import { route, setQuery, go } from '../router.js'
import { appId, appName, currentApp, revision, report } from '../store.js'
import { relative, region, shortId, fullTime } from '../format.js'
import { reasonName } from '../risk-display.js'
import RiskScore from '../components/RiskScore.vue'
import LevelTag from '../components/LevelTag.vue'

const LIMIT = 50
const listing = ref(null)
const loading = ref(false)
const search = ref(route.query.q || '')
const level = computed(() => route.query.level || '')
const sort = computed(() => route.query.sort || 'risk')
const offset = computed(() => Number(route.query.offset) || 0)

async function load() {
  loading.value = true
  try {
    listing.value = await api('devices' + query({ appId: appId.value, q: route.query.q, level: level.value, sort: sort.value, offset: offset.value, limit: LIMIT }))
  } catch (e) {
    report(e)
  } finally {
    loading.value = false
  }
}
watch([appId, () => route.query.q, level, sort, offset, revision], load, { immediate: true })

const stats = computed(() => listing.value?.stats)
const levels = [['', '全部'], ['high', '高风险'], ['medium', '中风险'], ['low', '低风险']]
const sorts = [['risk', '风险评分'], ['lastSeen', '最近活跃'], ['accounts', '关联账号'], ['events', '业务事件']]

function submit() { setQuery({ q: search.value.trim(), offset: '' }) }
function setLevel(value) { setQuery({ level: value, offset: '' }) }
function setSort(value) { setQuery({ sort: value, offset: '' }) }
function pageBy(delta) { setQuery({ offset: Math.max(0, offset.value + delta * LIMIT) || '' }) }

function model(p) { return p.latestReport.signals?.model?.value || '未识别机型' }
function brand(p) { return p.latestReport.signals?.manufacturer?.value || '' }
function envState(p) {
  if (p.keyStatus === 'revoked') return { text: '密钥已停用', cls: 'bad' }
  const checks = p.latestReport.resources?.checks
  if (!checks) return { text: '待比对', cls: '' }
  if (checks.some(c => c.state === 'conflict')) return { text: '参数矛盾', cls: 'bad' }
  if (checks.some(c => ['unknown', 'stale', 'error'].includes(c.state))) return { text: '部分比对', cls: '' }
  return { text: '比对一致', cls: 'good' }
}
</script>

<template>
  <div>
    <div class="page-head">
      <div><h1>设备</h1><p>{{ currentApp ? currentApp.name : '全部应用' }} · 按设备查看风险评分、关联账号与环境比对</p></div>
    </div>

    <div v-if="stats" class="grid g-kpi" style="margin-bottom:16px">
      <div class="card tile link" @click="setLevel('')"><div class="label">累计设备</div><div class="value">{{ stats.total }}</div><div class="delta">24 小时活跃 {{ stats.active }}</div></div>
      <div class="card tile link" @click="setLevel('high')"><div class="label"><i class="dot high"></i>高风险设备</div><div class="value">{{ stats.high }}</div><div class="delta">评分 ≥ 70</div></div>
      <div class="card tile link" @click="setLevel('medium')"><div class="label"><i class="dot medium"></i>中风险设备</div><div class="value">{{ stats.medium }}</div><div class="delta">评分 30–69</div></div>
      <div class="card tile link" @click="setLevel('low')"><div class="label"><i class="dot low"></i>低风险设备</div><div class="value">{{ stats.low }}</div><div class="delta">评分 &lt; 30</div></div>
      <div class="card tile"><div class="label">参数存在矛盾</div><div class="value">{{ stats.conflict }}</div><div class="delta">信息待补充 {{ stats.incomplete }}</div></div>
      <div class="card tile"><div class="label">密钥已停用</div><div class="value">{{ stats.revoked }}</div><div class="delta">停用后上报会被拒绝</div></div>
    </div>

    <section class="card" :class="{ loading }">
      <div class="filters">
        <form class="search" @submit.prevent="submit" style="display:flex;gap:8px">
          <input v-model="search" type="search" placeholder="设备 ID、机型、品牌或 IP" aria-label="搜索设备" style="flex:1">
          <button>搜索</button>
        </form>
        <div class="seg" aria-label="风险等级">
          <button v-for="[key, label] in levels" :key="key" :class="{ on: level === key }" @click="setLevel(key)">{{ label }}<span v-if="stats && key" class="n">{{ stats[key] }}</span></button>
        </div>
        <span style="flex:1"></span>
        <label class="row-gap muted" style="font-size:13px">排序
          <select :value="sort" @change="setSort($event.target.value)">
            <option v-for="[key, label] in sorts" :key="key" :value="key">{{ label }}</option>
          </select>
        </label>
      </div>
      <div class="table-wrap" v-if="listing">
        <table>
          <thead>
            <tr>
              <th :class="['sortable', { sorted: sort === 'risk' }]" @click="setSort('risk')">风险评分 {{ sort === 'risk' ? '↓' : '' }}</th>
              <th>设备</th>
              <th>主要风险线索</th>
              <th v-if="!appId">应用</th>
              <th :class="['sortable right', { sorted: sort === 'accounts' }]" @click="setSort('accounts')">关联账号 {{ sort === 'accounts' ? '↓' : '' }}</th>
              <th :class="['sortable right', { sorted: sort === 'events' }]" @click="setSort('events')">业务事件 {{ sort === 'events' ? '↓' : '' }}</th>
              <th>最近 IP</th>
              <th>环境比对</th>
              <th :class="['sortable', { sorted: sort === 'lastSeen' }]" @click="setSort('lastSeen')">最近活跃 {{ sort === 'lastSeen' ? '↓' : '' }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in listing.items" :key="p.id" class="click" @click="go('devices', p.id, { app: appId })">
              <td><span class="row-gap" style="flex-wrap:nowrap"><RiskScore :score="p.riskScore" /><LevelTag :level="p.riskLevel" /></span></td>
              <td><strong>{{ model(p) }}</strong><span class="sub">{{ brand(p) }} <span class="mono">{{ shortId(p.deviceId) || '未登记设备 ID' }}</span></span></td>
              <td>
                <span class="row-gap">
                  <span v-for="f in p.factors.slice(0, 2)" :key="f.code" class="tag">{{ reasonName(f.code) }}</span>
                  <span v-if="p.factors.length > 2" class="muted">+{{ p.factors.length - 2 }}</span>
                  <span v-if="!p.factors.length" class="muted">未命中</span>
                </span>
              </td>
              <td v-if="!appId">{{ appName(p.appId) }}</td>
              <td class="right num"><span :class="['tag', { warn: p.accountCount >= 3 }]">{{ p.accountCount }}</span></td>
              <td class="right num">{{ p.eventCount }}<span v-if="p.highEvents" class="sub" style="color:var(--high-ink)">高风险 {{ p.highEvents }}</span></td>
              <td><span class="mono">{{ p.lastIp || '—' }}</span><span class="sub">{{ region(p.region) }}</span></td>
              <td><span :class="['tag', envState(p).cls]">{{ envState(p).text }}</span></td>
              <td :title="fullTime(p.lastSeen)">{{ relative(p.lastSeen) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!listing.items.length" class="empty"><strong>没有符合条件的设备</strong>{{ route.query.q || level ? '调整搜索或筛选条件试试' : 'SDK 完成启动登记后，设备会出现在这里' }}</div>
      </div>
      <div class="pager" v-if="listing && listing.total > LIMIT">
        <span>{{ offset + 1 }}–{{ Math.min(offset + LIMIT, listing.total) }} / 共 {{ listing.total }} 台</span>
        <button class="small" :disabled="offset === 0" @click="pageBy(-1)">上一页</button>
        <button class="small" :disabled="offset + LIMIT >= listing.total" @click="pageBy(1)">下一页</button>
      </div>
    </section>
  </div>
</template>
