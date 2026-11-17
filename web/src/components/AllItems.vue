<script setup>
import { ref, computed } from 'vue'
import { reasonName, checkNames, checkStates, signalNames } from '../risk-display.js'
const props = defineProps({ report: { type: Object, required: true }, factors: { type: Array, default: () => [] } })
const query = ref('')
const statusText = { permission_denied: '未授权', unavailable: '不可用', disabled: '未开启', error: '采集出错' }

function flatten(value, path, out) {
  if (Array.isArray(value)) {
    if (value.every(v => v === null || typeof v !== 'object')) out.push([path, value.length ? value.join('、') : '空列表'])
    else value.forEach((v, i) => flatten(v, path + '[' + i + ']', out))
  } else if (value !== null && typeof value === 'object') {
    const keys = Object.keys(value)
    if (!keys.length) out.push([path, '空'])
    keys.forEach(k => flatten(value[k], path ? path + '.' + k : k, out))
  } else out.push([path, value])
}
const shown = v => (v === null || v === undefined || v === '' ? '空' : typeof v === 'boolean' ? (v ? '是' : '否') : String(v))

const checks = computed(() => {
  const hit = props.factors.filter(f => f.source === 'client').map(f => ({ code: f.code, name: reasonName(f.code), state: '命中', cls: 'bad', points: f.points, detail: '' }))
  const cls = { matched: 'good', conflict: 'bad', error: 'warn' }
  const resource = (props.report.resources?.checks || []).map(c => ({ code: c.code, name: checkNames[c.code] || c.code, state: checkStates[c.state] || c.state, cls: cls[c.state] || '', points: c.points, detail: c.detail }))
  return [...hit, ...resource]
})
const values = computed(() => {
  const rows = []
  for (const [key, s] of Object.entries(props.report.signals || {}).sort(([a], [b]) => a.localeCompare(b))) {
    const name = signalNames[key] || key
    if (s.status !== 'available') { rows.push({ key, name, field: '', value: statusText[s.status] || s.status, note: s.reason }); continue }
    const out = []
    flatten(s.value, '', out)
    out.forEach(([field, value]) => rows.push({ key, name, field, value: shown(value) }))
  }
  return rows
})
const match = (...parts) => { const q = query.value.trim().toLowerCase(); return !q || parts.some(p => String(p ?? '').toLowerCase().includes(q)) }
const shownChecks = computed(() => checks.value.filter(c => match(c.code, c.name, c.state, c.detail)))
const shownValues = computed(() => values.value.filter(v => match(v.key, v.name, v.field, v.value, v.note)))
</script>

<template>
  <section class="card">
    <div class="filters">
      <input v-model="query" class="search" type="search" placeholder="按名称、字段或值筛选，如 port、frida、460" aria-label="筛选条目">
      <span class="muted">{{ shownChecks.length }} 个检测项 · {{ shownValues.length }} 个采集值</span>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>检测项</th><th>规则代码</th><th>结果</th><th class="right">分值</th><th>说明</th></tr></thead>
        <tbody>
          <tr v-for="(c, i) in shownChecks" :key="'c' + i"><td>{{ c.name }}</td><td class="mono">{{ c.code }}</td><td><span :class="['tag', c.cls]">{{ c.state }}</span></td><td class="right num">{{ c.points ? '+' + c.points : '' }}</td><td style="overflow-wrap:anywhere">{{ c.detail }}</td></tr>
        </tbody>
      </table>
      <div v-if="!shownChecks.length" class="empty">没有匹配的检测项</div>
    </div>
    <div class="table-wrap" style="border-top:1px solid var(--line)">
      <table>
        <thead><tr><th>采集项</th><th>字段</th><th>值</th></tr></thead>
        <tbody>
          <tr v-for="(v, i) in shownValues" :key="'v' + i"><td>{{ v.name }} <small class="muted mono">{{ v.key }}</small></td><td class="mono">{{ v.field }}</td><td class="mono" style="overflow-wrap:anywhere;white-space:pre-wrap">{{ v.value }}<small v-if="v.note" class="muted"> · {{ v.note }}</small></td></tr>
        </tbody>
      </table>
      <div v-if="!shownValues.length" class="empty">没有匹配的采集值</div>
    </div>
    <p class="muted" style="margin:0;padding:12px 16px;font-size:13px">设备环境规则只记录命中项；未出现在表中的规则表示本次上报未命中或缺少对应数据。</p>
  </section>
</template>
