<script setup>
import { ref, computed, watch } from 'vue'
import { api, query } from '../api.js'
import { route, setQuery, go } from '../router.js'
import { appId, currentApp, revision, report, appName } from '../store.js'
import { levelNames, relative } from '../format.js'
import { reasonName } from '../risk-display.js'
import BucketBars from '../components/BucketBars.vue'
import ChinaTiles from '../components/ChinaTiles.vue'
import Heatmap from '../components/Heatmap.vue'
import RiskScore from '../components/RiskScore.vue'

const ranges = [['24h', '24 小时'], ['7d', '7 天'], ['30d', '30 天']]
const range = computed(() => (ranges.some(r => r[0] === route.query.range) ? route.query.range : '7d'))
const unit = computed(() => (route.query.unit === 'handset' ? 'handset' : 'device'))
const unitText = computed(() => (unit.value === 'handset' ? '台手机' : '个设备'))
const data = ref(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    data.value = await api('insights' + query({ appId: appId.value, range: range.value }))
  } catch (e) {
    report(e)
  } finally {
    loading.value = false
  }
}
watch([appId, range, revision], load, { immediate: true })

// Filters live in the URL as JSON {dim: value}, so a drill-down is shareable.
const filters = computed(() => {
  try {
    const f = JSON.parse(route.query.f || '{}')
    return f && typeof f === 'object' ? f : {}
  } catch {
    return {}
  }
})
function toggle(dim, value) {
  const f = { ...filters.value }
  if (f[dim] === value) delete f[dim]
  else f[dim] = value
  setQuery({ f: Object.keys(f).length ? JSON.stringify(f) : '' })
}

const dims = computed(() => data.value?.dims || [])
const dimIndex = computed(() => Object.fromEntries(dims.value.map((d, i) => [d.key, i])))
const valueNames = { level: v => levelNames[v] || v, factor: reasonName, proof: v => ({ strongbox: 'StrongBox 硬件证明', tee: 'TEE 硬件证明', signature_only: '仅签名（无硬件证明）' })[v] || v }
const label = (dim, v) => (valueNames[dim] ? valueNames[dim](v) : v)

// Rows matching every filter except `skip` (crossfilter: a card is counted
// with the other cards' filters, so its own siblings stay visible).
function matching(skip) {
  const active = Object.entries(filters.value).filter(([k]) => k !== skip && k in dimIndex.value)
  if (!active.length) return data.value?.rows || []
  const tests = active.map(([k, v]) => {
    const i = dimIndex.value[k], code = dims.value[i].values.indexOf(v)
    return dims.value[i].multi ? r => r.d[i].includes(code) : r => r.d[i] === code
  })
  return data.value.rows.filter(r => tests.every(t => t(r)))
}

// Counting unit: one profile, or one handset (reinstalls and apps merged,
// its risk is the highest of its profiles).
function units(rows) {
  if (unit.value === 'device') return rows.map(r => ({ score: r.score, rows: [r] }))
  const by = new Map()
  for (const r of rows) {
    const u = by.get(r.handset)
    if (u) { u.score = Math.max(u.score, r.score); u.rows.push(r) } else by.set(r.handset, { score: r.score, rows: [r] })
  }
  return [...by.values()]
}
const levelOf = s => (s >= 70 ? 'high' : s >= 30 ? 'medium' : 'low')

function buckets(dim) {
  const i = dimIndex.value[dim], d = dims.value[i]
  const counts = new Map()
  for (const u of units(matching(dim))) {
    const codes = new Set()
    for (const r of u.rows) for (const c of d.multi ? r.d[i] : [r.d[i]]) codes.add(c)
    const level = levelOf(u.score)
    for (const c of codes) {
      const b = counts.get(c) || { count: 0, low: 0, medium: 0, high: 0 }
      b.count++
      b[level]++
      counts.set(c, b)
    }
  }
  return [...counts].map(([c, b]) => ({ value: d.values[c], label: label(dim, d.values[c]), ...b }))
    .sort((a, b) => (a.value === '未知') - (b.value === '未知') || b.count - a.count || a.label.localeCompare(b.label))
}

const filtered = computed(() => units(matching(null)))
const kpis = computed(() => {
  const all = filtered.value, n = all.length
  const high = all.filter(u => u.score >= 70).length, medium = all.filter(u => u.score >= 30 && u.score < 70).length
  const pct = x => (n ? Math.round((x / n) * 100) : 0) + '%'
  const provinces = data.value ? buckets('province').filter(b => b.value !== '未知').length : 0
  const factor = data.value ? buckets('factor')[0] : null
  return [
    { label: unit.value === 'handset' ? '手机' : '设备', value: n.toLocaleString('zh-CN') },
    { label: '高风险', value: high.toLocaleString('zh-CN'), sub: pct(high), dot: 'high' },
    { label: '中风险', value: medium.toLocaleString('zh-CN'), sub: pct(medium), dot: 'medium' },
    { label: '平均风险分', value: n ? Math.round(all.reduce((s, u) => s + u.score, 0) / n) : 0, sub: '/ 100' },
    { label: '覆盖地区', value: provinces, sub: '个省份 / 境外' },
    { label: '最常见线索', value: factor ? factor.label : '—', sub: factor ? factor.count + ' ' + unitText.value : '', small: true },
  ]
})

// Province map: IP regions carry full names (广东省); tiles use short ones.
const shortName = p => p.replace(/(省|市|壮族自治区|回族自治区|维吾尔自治区|自治区|特别行政区)$/, '')
const provinceRows = computed(() => (data.value ? buckets('province') : []))
const tileValues = computed(() => Object.fromEntries(provinceRows.value.map(b => [shortName(b.value), { count: b.count, high: b.high, raw: b.value }])))
const mapMetric = ref('count')

const groups = [
  { key: 'risk', title: '风险构成', dims: ['level', 'factor', 'proof'] },
  { key: 'geo', title: '地域', dims: ['city', 'isp'] },
  { key: 'network', title: '网络', dims: ['network', 'sim', 'proxy'] },
  { key: 'device', title: '设备', dims: ['brand', 'model', 'android', 'memory', 'screen', 'patch', 'appVersion'] },
  { key: 'habit', title: '使用状态', dims: ['battery', 'uptime', 'age', 'accounts'] },
]
const dimLabel = k => dims.value[dimIndex.value[k]]?.label || k
const hints = {
  battery: '整机架长期插电满电是挂机特征',
  uptime: '长时间不重启常见于机房设备',
  accounts: '一台设备登录多个账号',
  proxy: '系统级 HTTP 代理',
  factor: '一个设备可命中多条线索',
  network: '盒子 / 云手机多为有线网卡',
}

const topRows = computed(() => matching(null).slice(0, 100))
const filterChips = computed(() => Object.entries(filters.value).map(([k, v]) => ({ k, v, text: dimLabel(k) + '：' + label(k, v) })))
</script>

<template>
  <div :class="{ loading }">
    <div class="page-head">
      <div>
        <h1>大盘分析</h1>
        <p>{{ currentApp ? currentApp.name : '全部应用' }} · 最近 {{ ranges.find(r => r[0] === range)[1] }}内活跃的设备 · 点任意分桶即可下钻，其他面板同步筛选</p>
      </div>
      <div class="row-gap">
        <div class="seg" role="tablist" aria-label="计数单位">
          <button :class="{ on: unit === 'device' }" @click="setQuery({ unit: '' })">按设备</button>
          <button :class="{ on: unit === 'handset' }" title="同一台手机的重装、多个应用合并为一台" @click="setQuery({ unit: 'handset' })">按手机</button>
        </div>
        <div class="seg" role="tablist" aria-label="时间范围">
          <button v-for="[key, text] in ranges" :key="key" :class="{ on: range === key }" @click="setQuery({ range: key })">{{ text }}</button>
        </div>
      </div>
    </div>

    <template v-if="data">
      <div v-if="filterChips.length" class="card" style="margin-bottom:16px;padding:10px 14px;display:flex;flex-wrap:wrap;gap:8px;align-items:center">
        <span class="muted" style="font-size:13px">筛选</span>
        <button v-for="c in filterChips" :key="c.k" class="chip" @click="toggle(c.k, c.v)">{{ c.text }} <span aria-hidden="true">×</span></button>
        <button class="small" style="margin-left:auto" @click="setQuery({ f: '' })">清除全部</button>
      </div>

      <div class="grid g-kpi" style="margin-bottom:16px">
        <div v-for="t in kpis" :key="t.label" class="card tile">
          <div class="label"><i v-if="t.dot" :class="['dot', t.dot]"></i>{{ t.label }}</div>
          <div class="value" :style="t.small ? 'font-size:17px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis' : ''" :title="String(t.value)">{{ t.value }}</div>
          <div class="delta">{{ t.sub }}</div>
        </div>
      </div>

      <div class="grid g-2-1 stack" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head">
            <div><h2>地域分布</h2><span class="sub">按出口 IP 归属省份 · 点击省份筛选</span></div>
            <div class="seg">
              <button :class="{ on: mapMetric === 'count' }" @click="mapMetric = 'count'">数量</button>
              <button :class="{ on: mapMetric === 'highShare' }" @click="mapMetric = 'highShare'">高风险占比</button>
            </div>
          </div>
          <div class="card-body">
            <ChinaTiles :values="tileValues" :metric="mapMetric" :selected="filters.province ? shortName(filters.province) : ''" @select="name => toggle('province', tileValues[name].raw)" />
          </div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>省份</h2><span class="sub">{{ unitText }} · 右侧为高风险占比</span></div></div>
          <div class="card-body" style="max-height:440px;overflow:auto"><BucketBars :rows="provinceRows" :selected="filters.province ?? null" :limit="14" @select="v => toggle('province', v)" /></div>
        </section>
      </div>

      <template v-for="g in groups" :key="g.key">
        <h3 class="section-title">{{ g.title }}</h3>
        <div class="grid g-3" style="margin-bottom:16px">
          <section v-for="k in g.dims" :key="k" class="card">
            <div class="card-head"><div><h2>{{ dimLabel(k) }}</h2><span class="sub">{{ hints[k] || unitText }}</span></div></div>
            <div class="card-body"><BucketBars :rows="buckets(k)" :selected="filters[k] ?? null" @select="v => toggle(k, v)" /></div>
          </section>
          <section v-if="g.key === 'habit'" class="card wide">
            <div class="card-head"><div><h2>活跃时段</h2><span class="sub">按服务器本地时间 · 不受上方筛选影响</span></div></div>
            <div class="card-body"><Heatmap :grid="data.activity" /></div>
          </section>
        </div>
      </template>

      <section class="card">
        <div class="card-head"><div><h2>命中设备</h2><span class="sub">当前筛选下风险最高的 {{ topRows.length }} 个设备</span></div></div>
        <div class="table-wrap" style="max-height:520px;overflow:auto">
          <table v-if="topRows.length">
            <thead><tr><th>设备</th><th v-if="!appId">应用</th><th>风险评分</th><th>地区</th><th>网络</th><th>主要线索</th><th>最近活跃</th></tr></thead>
            <tbody>
              <tr v-for="r in topRows" :key="r.id" class="click" @click="go('devices', r.id, { app: appId })">
                <td><strong>{{ dims[dimIndex.model].values[r.d[dimIndex.model]] }}</strong></td>
                <td v-if="!appId">{{ appName(r.appId) }}</td>
                <td><RiskScore :score="r.score" /></td>
                <td>{{ dims[dimIndex.city].values[r.d[dimIndex.city]] === '未知' ? dims[dimIndex.province].values[r.d[dimIndex.province]] : dims[dimIndex.city].values[r.d[dimIndex.city]] }}</td>
                <td>{{ dims[dimIndex.network].values[r.d[dimIndex.network]] }}</td>
                <td><span class="row-gap"><span v-for="c in r.d[dimIndex.factor].slice(0, 2)" :key="c" class="tag">{{ reasonName(dims[dimIndex.factor].values[c]) }}</span><span v-if="!r.d[dimIndex.factor].length" class="muted">—</span></span></td>
                <td>{{ relative(r.lastSeen) }}</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="empty">当前筛选下没有设备</div>
        </div>
      </section>
      <p v-if="data.truncated" class="muted" style="font-size:13px">设备过多，只统计风险最高的 50,000 个。</p>
    </template>
    <div v-else class="empty">加载中…</div>
  </div>
</template>
