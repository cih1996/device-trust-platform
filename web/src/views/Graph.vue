<script setup>
import { ref, computed, watch } from 'vue'
import { api, query } from '../api.js'
import { route, setQuery, go } from '../router.js'
import { appId, currentApp, revision, report, appName } from '../store.js'
import ForceGraph from '../components/ForceGraph.vue'

const kinds = {
  handset: { label: '同一台手机', color: 'var(--s1)', weight: 1 },
  account: { label: '共用账号', color: 'var(--s2)', weight: 0.8 },
  lan: { label: '同一局域网', color: 'var(--s3)', weight: 0.6 },
  cell: { label: '同小时同基站', color: 'var(--s4)', weight: 0.4 },
  ip: { label: '同日同宽带出口', color: 'var(--muted)', dash: '4 3', weight: 0.3 },
}
const ranges = [['1', '1 天'], ['7', '7 天'], ['30', '30 天']]
const days = computed(() => (ranges.some(r => r[0] === route.query.days) ? route.query.days : '7'))
const selected = computed(() => Number(route.query.cluster) || 0)
const data = ref(null)

async function load() {
  try {
    data.value = await api('graph' + query({ appId: appId.value, days: days.value }))
  } catch (e) {
    report(e)
  }
}
watch([appId, days, revision], load, { immediate: true })

const nodes = computed(() => (data.value?.nodes || []).filter(n => !selected.value || n.cluster === selected.value).map(n => ({ ...n, appName: appName(n.appId) })))
const edges = computed(() => {
  const ids = new Set(nodes.value.map(n => n.id))
  return (data.value?.edges || []).filter(e => ids.has(e.source) && ids.has(e.target))
})
const byId = computed(() => new Map((data.value?.nodes || []).map(n => [n.id, n])))
const stats = computed(() => {
  const c = data.value?.clusters || []
  return [
    ['关联团伙', c.length],
    ['涉及设备', c.reduce((s, x) => s + x.size, 0)],
    ['最大团伙', c.length ? c[0].size + ' 台' : '—'],
    ['含高风险设备的团伙', c.filter(x => x.highRisk > 0).length],
  ]
})
const kindSummary = c => Object.entries(c.kinds).sort((a, b) => kinds[b[0]].weight - kinds[a[0]].weight).map(([k, n]) => kinds[k].label + ' ' + n)
const label = id => {
  const n = byId.value.get(id)
  return n ? n.model + (n.lanIp ? ' · ' + n.lanIp : '') : id.slice(0, 8)
}
function open(id) {
  go('devices', id, { app: appId.value })
}
</script>

<template>
  <div>
    <div class="page-head">
      <div>
        <h1>关系图谱</h1>
        <p>{{ currentApp ? currentApp.name : '全部应用' }} · 最近 {{ ranges.find(r => r[0] === days)[1] }} · 关联系数 ≥ 0.5 的设备归为同一团伙</p>
      </div>
      <div class="seg" role="tablist" aria-label="时间范围">
        <button v-for="[key, text] in ranges" :key="key" :class="{ on: days === key }" @click="setQuery({ days: key, cluster: '' })">{{ text }}</button>
      </div>
    </div>

    <template v-if="data">
      <div class="grid g-4" style="margin-bottom:16px">
        <div v-for="[name, value] in stats" :key="name" class="card tile"><div class="label">{{ name }}</div><div class="value">{{ value }}</div></div>
      </div>

      <section class="card" style="margin-bottom:16px">
        <div class="card-head">
          <div><h2>{{ selected ? '团伙 #' + selected : '全部团伙' }}</h2><span class="sub">点圆点进入设备；线越粗关联越强，颜色为最强的关联依据</span></div>
          <button v-if="selected" class="small" @click="setQuery({ cluster: '' })">查看全部</button>
        </div>
        <div class="card-body">
          <div class="legend" style="margin-bottom:12px">
            <span v-for="(k, key) in kinds" :key="key"><i :style="{ height: '3px', width: '16px', background: k.dash ? 'repeating-linear-gradient(90deg,' + k.color + ' 0 4px,transparent 4px 7px)' : k.color }"></i>{{ k.label }}（{{ k.weight }}）</span>
            <span><i style="background:var(--high);border-radius:50%"></i>高风险</span>
            <span><i style="background:var(--medium);border-radius:50%"></i>中风险</span>
            <span><i style="background:var(--low);border-radius:50%"></i>低风险</span>
          </div>
          <ForceGraph v-if="nodes.length" :nodes="nodes" :edges="edges" :kinds="kinds" @open="n => open(n.id)" />
          <div v-else class="empty">这段时间没有发现关联设备</div>
          <p v-if="data.trimmed" class="muted" style="font-size:13px;margin:8px 0 0">设备较多，只显示最大的若干团伙（最多 600 台）。</p>
          <p class="muted" style="font-size:13px;margin:8px 0 0">关联系数 = 1 − ∏(1 − 各依据权重)。同出口 IP 只计宽带，移动网络 NAT 出口会被陌生人共用；成员过多的分组（如热门基站）不参与关联<template v-if="Object.keys(data.skippedGroups).length">，本次跳过 {{ Object.entries(data.skippedGroups).map(([k, n]) => kinds[k].label + ' ' + n + ' 组').join('、') }}</template>。</p>
        </div>
      </section>

      <div class="grid g-2 stack">
        <section class="card">
          <div class="card-head"><h2>团伙列表</h2><span class="sub">{{ data.clusters.length }} 个，按规模排序</span></div>
          <div class="table-wrap" style="max-height:480px;overflow:auto">
            <table v-if="data.clusters.length">
              <thead><tr><th>团伙</th><th class="right">设备</th><th>关联依据（条）</th><th class="right">最强系数</th><th class="right">最高风险分</th><th class="right">应用</th></tr></thead>
              <tbody>
                <tr v-for="c in data.clusters" :key="c.id" :class="['click', { on: selected === c.id }]" @click="setQuery({ cluster: c.id })">
                  <td>#{{ c.id }}</td><td class="right num">{{ c.size }}</td><td>{{ kindSummary(c).join('、') }}</td>
                  <td class="right num">{{ c.strongest.toFixed(2) }}</td>
                  <td class="right num"><span :class="['tag', { bad: c.highRisk > 0 }]">{{ c.maxRisk }}</span></td><td class="right num">{{ c.apps }}</td>
                </tr>
              </tbody>
            </table>
            <div v-else class="empty">没有团伙</div>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><h2>关联明细</h2><span class="sub">{{ selected ? '团伙 #' + selected : '全部' }} · 前 100 条</span></div>
          <div class="table-wrap" style="max-height:480px;overflow:auto">
            <table v-if="edges.length">
              <thead><tr><th>设备 A</th><th>设备 B</th><th class="right">系数</th><th>依据</th></tr></thead>
              <tbody>
                <tr v-for="(e, i) in edges.slice(0, 100)" :key="i">
                  <td><a href="#" @click.prevent="open(e.source)">{{ label(e.source) }}</a></td>
                  <td><a href="#" @click.prevent="open(e.target)">{{ label(e.target) }}</a></td>
                  <td class="right num">{{ e.score.toFixed(2) }}</td>
                  <td style="white-space:normal">{{ e.reasons.map(r => r.detail).join('；') }}</td>
                </tr>
              </tbody>
            </table>
            <div v-else class="empty">没有关联</div>
          </div>
        </section>
      </div>
    </template>
    <div v-else class="empty">加载关系图谱…</div>
  </div>
</template>
