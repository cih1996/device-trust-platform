<script setup>
import { ref, computed, watch } from 'vue'
import { api, query } from '../api.js'
import { route, setQuery, go } from '../router.js'
import { appId, currentApp, revision, report, openEvent, appName } from '../store.js'
import { compact, dateTime, region, shortId, relative } from '../format.js'
import { sceneNames, reasonName, statusNames } from '../risk-display.js'
import StackedColumns from '../components/StackedColumns.vue'
import LineChart from '../components/LineChart.vue'
import Donut from '../components/Donut.vue'
import BarList from '../components/BarList.vue'
import RiskScore from '../components/RiskScore.vue'
import LevelTag from '../components/LevelTag.vue'

const ranges = [['24h', '24 小时'], ['7d', '7 天'], ['30d', '30 天']]
const range = computed(() => route.query.range || '24h')
const data = ref(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    data.value = await api('analytics' + query({ appId: appId.value, range: range.value }))
  } catch (e) {
    report(e)
  } finally {
    loading.value = false
  }
}
watch([appId, range, revision], load, { immediate: true })

const step = computed(() => (data.value?.step === '1h0m0s' ? 'hour' : 'day'))
const rangeLabel = computed(() => ranges.find(r => r[0] === range.value)[1])
const riskKeys = [
  { key: 'low', label: '低风险', color: 'var(--low)' },
  { key: 'medium', label: '中风险', color: 'var(--medium)' },
  { key: 'high', label: '高风险', color: 'var(--high)' },
]
const sceneColors = { registration: 'var(--s1)', marketing: 'var(--s2)', payment: 'var(--s3)', general: 'var(--s4)' }

function delta(key, upIsBad) {
  const now = data.value.summary[key], before = data.value.previous[key]
  if (!before && !now) return { text: '与上期持平', cls: 'flat' }
  if (!before) return { text: '上期无数据', cls: 'flat' }
  const pct = Math.round(((now - before) / before) * 100)
  if (pct === 0) return { text: '与上期持平', cls: 'flat' }
  const up = pct > 0
  return { text: (up ? '↑ ' : '↓ ') + Math.abs(pct) + '% 较上期', cls: upIsBad == null ? 'flat' : up === upIsBad ? 'up-bad' : 'up-good' }
}
const tiles = computed(() => {
  if (!data.value) return []
  const s = data.value.summary
  return [
    { label: '业务事件', value: compact(s.events), delta: delta('events'), to: () => go('events', '', { app: appId.value, range: range.value }) },
    { label: '活跃设备', value: compact(s.devices), delta: { text: '新设备 ' + s.newDevices, cls: 'flat' }, to: () => go('devices', '', { app: appId.value }) },
    { label: '高风险事件', value: compact(s.highRisk), delta: delta('highRisk', true), dot: 'high', to: () => go('events', '', { app: appId.value, range: range.value, level: 'high' }) },
    { label: '建议核验', value: compact(s.review), delta: delta('review', true), to: () => go('events', '', { app: appId.value, range: range.value, level: 'medium' }) },
    { label: '平均风险分', value: s.averageRisk, unit: '/ 100', delta: delta('averageRisk', true) },
    { label: '关联账号', value: compact(s.accounts), delta: { text: '独立 IP ' + s.ips + (s.dwellSamples ? ' · 平均停留 ' + s.dwellSeconds + ' 秒' : ''), cls: 'flat' } },
  ]
})
const deviceSegments = computed(() => riskKeys.map(k => ({ ...k, value: data.value?.deviceLevels[k.key] || 0 })))
const sceneRows = computed(() => Object.entries(sceneNames).map(([key, label]) => ({ key, label, value: data.value?.scenes[key] || 0, color: sceneColors[key] })))
const reasonRows = computed(() => (data.value?.reasons || []).map(r => ({ key: r.key, label: reasonName(r.key), value: r.count })))
const regionRows = computed(() => (data.value?.regions || []).map(r => ({ key: r.key, label: r.key, value: r.count })))
const brandRows = computed(() => (data.value?.brands || []).map(r => ({ key: r.key, label: r.key, value: r.count })))
const hasEvents = computed(() => data.value?.summary.events > 0)
</script>

<template>
  <div :class="{ loading }">
    <div class="page-head">
      <div>
        <h1>风险总览</h1>
        <p>{{ currentApp ? currentApp.name : '全部应用' }} · 最近 {{ rangeLabel }}</p>
      </div>
      <div class="seg" role="tablist" aria-label="时间范围">
        <button v-for="[key, label] in ranges" :key="key" :class="{ on: range === key }" @click="setQuery({ range: key })">{{ label }}</button>
      </div>
    </div>

    <template v-if="data">
      <div class="grid g-kpi" style="margin-bottom:16px">
        <div v-for="t in tiles" :key="t.label" :class="['card tile', { link: t.to }]" @click="t.to && t.to()">
          <div class="label"><i v-if="t.dot" :class="['dot', t.dot]"></i>{{ t.label }}</div>
          <div class="value">{{ t.value }}<small v-if="t.unit">{{ t.unit }}</small></div>
          <div class="delta"><span :class="t.delta.cls">{{ t.delta.text }}</span></div>
        </div>
      </div>

      <div class="grid g-2-1" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head">
            <div><h2>业务事件趋势</h2><span class="sub">按{{ step === 'hour' ? '小时' : '天' }}统计，按风险等级拆分</span></div>
            <div class="legend"><span v-for="k in riskKeys" :key="k.key"><i :style="{ background: k.color }"></i>{{ k.label }}</span></div>
          </div>
          <div class="card-body"><StackedColumns :points="data.series" :keys="riskKeys" :step="step" :height="240" /></div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>设备风险分布</h2><span class="sub">期间活跃设备的当前评分</span></div></div>
          <div class="card-body">
            <Donut :segments="deviceSegments" center-label="活跃设备" @select="level => go('devices', '', { app: appId, level })" />
            <p class="muted" style="font-size:12px;margin:14px 0 0">设备评分 = 最新上报的环境评分 + 7 天内业务行为最高分（上限 100）。环境恢复正常后，下次上报即降分。</p>
          </div>
        </section>
      </div>

      <div class="grid g-3" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><div><h2>平均风险分</h2><span class="sub">无事件的时段留空</span></div></div>
          <div class="card-body"><LineChart :points="data.series" field="averageRisk" empty-as="events" label="平均风险分" :max="100" :step="step" color="var(--high)" /></div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>活跃设备</h2><span class="sub">有上报的设备数</span></div></div>
          <div class="card-body"><LineChart :points="data.series" field="devices" label="活跃设备" unit=" 台" :step="step" /></div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>业务场景</h2><span class="sub">事件数</span></div></div>
          <div class="card-body"><BarList :rows="sceneRows" clickable @select="r => go('events', '', { app: appId, range, scene: r.key })" /></div>
        </section>
      </div>

      <div class="grid g-2-1" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head">
            <div><h2>高风险设备</h2><span class="sub">按设备风险评分排序</span></div>
            <a :href="'#/devices?sort=risk' + (appId ? '&app=' + appId : '')">查看全部设备 →</a>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>设备</th><th>风险评分</th><th>主要线索</th><th class="right">事件</th><th class="right">账号</th><th>最近活跃</th></tr></thead>
              <tbody>
                <tr v-for="d in data.topDevices" :key="d.id" class="click" @click="go('devices', d.id, { app: appId })">
                  <td><strong>{{ d.model || '未识别机型' }}</strong><span class="sub mono">{{ shortId(d.deviceId) || '未登记' }}</span></td>
                  <td><RiskScore :score="d.riskScore" /></td>
                  <td><span class="row-gap"><span v-for="f in d.factors.slice(0, 2)" :key="f.code" class="tag">{{ reasonName(f.code) }}</span><span v-if="!d.factors.length" class="muted">—</span></span></td>
                  <td class="right num">{{ d.events }}</td>
                  <td class="right num">{{ d.accounts }}</td>
                  <td>{{ relative(d.lastSeen) }}</td>
                </tr>
              </tbody>
            </table>
            <div v-if="!data.topDevices.length" class="empty">期间没有活跃设备</div>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>风险线索排行</h2><span class="sub">命中事件数</span></div></div>
          <div class="card-body"><BarList :rows="reasonRows" empty="期间没有命中规则" /></div>
        </section>
      </div>

      <div class="grid g-2-1" style="margin-bottom:16px">
        <section class="card">
          <div class="card-head"><div><h2>访问 IP</h2><span class="sub">业务事件来源 IP，按访问次数排序</span></div></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>IP</th><th>归属</th><th class="right">访问</th><th class="right">设备</th><th class="right">账号</th><th>最高风险</th></tr></thead>
              <tbody>
                <tr v-for="ip in data.ips" :key="ip.ip" class="click" @click="go('events', '', { app: appId, range, q: ip.ip })">
                  <td class="mono">{{ ip.ip }}</td>
                  <td>{{ region(ip.region) || '未匹配' }}<span v-if="ip.region?.isp" class="sub">{{ ip.region.isp }}</span></td>
                  <td class="right num">{{ ip.count }}</td>
                  <td class="right num"><span :class="{ tag: true, warn: ip.devices >= 3 }">{{ ip.devices }}</span></td>
                  <td class="right num"><span :class="{ tag: true, warn: ip.accounts >= 3 }">{{ ip.accounts }}</span></td>
                  <td><RiskScore :score="ip.maxScore" /></td>
                </tr>
              </tbody>
            </table>
            <div v-if="!data.ips.length" class="empty">期间没有带 IP 的业务事件</div>
          </div>
        </section>
        <section class="card">
          <div class="card-head"><div><h2>设备品牌</h2><span class="sub">活跃设备</span></div></div>
          <div class="card-body"><BarList :rows="brandRows" empty="暂无设备" /></div>
          <template v-if="regionRows.length">
            <div class="card-head" style="padding-top:4px"><div><h2>访问地区</h2><span class="sub">按 IP 归属省份</span></div></div>
            <div class="card-body"><BarList :rows="regionRows" /></div>
          </template>
        </section>
      </div>

      <section class="card">
        <div class="card-head">
          <div><h2>最近风险事件</h2><span class="sub">风险评分 ≥ 30</span></div>
          <a :href="'#/events?level=high' + (appId ? '&app=' + appId : '')">查看全部事件 →</a>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>时间</th><th>账号 / 场景</th><th v-if="!appId">应用</th><th>风险评分</th><th>命中线索</th><th>处置</th></tr></thead>
            <tbody>
              <tr v-for="e in data.recent" :key="e.id" class="click" @click="openEvent(e.id)">
                <td>{{ dateTime(e.receivedAt) }}</td>
                <td>{{ e.accountId }}<span class="sub">{{ sceneNames[e.scene] || e.type }}</span></td>
                <td v-if="!appId">{{ appName(e.appId) }}</td>
                <td><RiskScore :score="e.score" /></td>
                <td><span class="row-gap"><span v-for="h in e.hits.slice(0, 2)" :key="h.code" class="tag">{{ reasonName(h.code) }}</span><span v-if="e.hits.length > 2" class="muted">+{{ e.hits.length - 2 }}</span></span></td>
                <td><span :class="['tag', { warn: e.status === 'pending' }]">{{ statusNames[e.status] }}</span></td>
              </tr>
            </tbody>
          </table>
          <div v-if="!data.recent.length" class="empty"><strong>{{ hasEvents ? '期间没有中高风险事件' : '期间暂无业务事件' }}</strong>{{ hasEvents ? '' : '应用接入后，业务事件会显示在这里' }}</div>
        </div>
      </section>
    </template>
  </div>
</template>
