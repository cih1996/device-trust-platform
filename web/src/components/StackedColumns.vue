<script setup>
import { computed, ref } from 'vue'
import { useWidth, niceTicks, axisLabel, bucketLabel, columnPath } from './chart-util.js'

const props = defineProps({
  points: { type: Array, required: true },
  keys: { type: Array, required: true }, // [{ key, label, color }] bottom → top
  step: { type: String, default: 'hour' },
  height: { type: Number, default: 220 },
  unit: { type: String, default: '次' },
})
const emit = defineEmits(['select'])
const { el, width } = useWidth()
const hover = ref(-1)
const pad = { top: 10, right: 8, bottom: 24, left: 36 }

const layout = computed(() => {
  const n = props.points.length || 1
  const totals = props.points.map(p => props.keys.reduce((s, k) => s + (p[k.key] || 0), 0))
  const ticks = niceTicks(Math.max(...totals, 0))
  const top = ticks[ticks.length - 1]
  const plotW = width.value - pad.left - pad.right
  const plotH = props.height - pad.top - pad.bottom
  const band = plotW / n
  const barW = Math.max(2, Math.min(24, band * 0.62))
  const y = v => pad.top + plotH - (v / top) * plotH
  const labelEvery = Math.ceil(n / Math.max(1, Math.floor(plotW / 48)))
  const columns = props.points.map((p, i) => {
    const x = pad.left + band * i + (band - barW) / 2
    let base = 0
    const visible = props.keys.filter(k => p[k.key] > 0)
    const segments = visible.map((k, j) => {
      const v = p[k.key]
      const y0 = y(base), y1 = y(base + v)
      base += v
      const gap = j < visible.length - 1 ? 2 : 0 // surface gap between stacked segments
      const h = Math.max(0, y0 - y1 - gap)
      const isTop = j === visible.length - 1
      return { key: k.key, color: k.color, d: isTop ? columnPath(x, y1, barW, h) : `M${x},${y1 + gap}h${barW}v${h}h${-barW}Z` }
    })
    return { i, x, band: pad.left + band * i, bandW: band, segments, total: totals[i], label: i % labelEvery === 0 ? bucketLabel(p.start, props.step) : '' }
  })
  return { ticks: ticks.map(t => ({ v: t, y: y(t) })), columns, plotH }
})

const tip = computed(() => {
  if (hover.value < 0) return null
  const c = layout.value.columns[hover.value]
  const p = props.points[hover.value]
  const left = Math.min(c.band + c.bandW / 2 + 12, width.value - 170)
  return { left, p, total: c.total, title: tipTitle(p.start) }
})
function tipTitle(start) {
  const d = new Date(start)
  return props.step === 'hour'
    ? d.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' }) + ' ' + String(d.getHours()).padStart(2, '0') + ':00'
    : d.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric', weekday: 'short' })
}
</script>

<template>
  <div class="chart" ref="el" @mouseleave="hover = -1">
    <svg :width="width" :height="height" role="img" :aria-label="'按' + (step === 'hour' ? '小时' : '天') + '统计的柱状图'">
      <g>
        <template v-for="t in layout.ticks" :key="t.v">
          <line :class="t.v === 0 ? 'baseline' : 'gridline'" :x1="pad.left" :x2="width - pad.right" :y1="t.y" :y2="t.y" />
          <text class="axis" :x="pad.left - 6" :y="t.y + 4" text-anchor="end">{{ axisLabel(t.v) }}</text>
        </template>
      </g>
      <g v-for="c in layout.columns" :key="c.i">
        <rect v-if="hover === c.i" :x="c.band" :y="pad.top" :width="c.bandW" :height="layout.plotH" fill="var(--surface-hover)" />
        <path v-for="s in c.segments" :key="s.key" :d="s.d" :fill="s.color" />
        <text v-if="c.label" class="axis" :x="c.x + Math.min(24, c.bandW * .62) / 2" :y="height - 6" text-anchor="middle">{{ c.label }}</text>
        <rect :x="c.band" :y="pad.top" :width="c.bandW" :height="layout.plotH" fill="transparent" style="cursor:pointer"
          @mouseenter="hover = c.i" @click="emit('select', points[c.i])" />
      </g>
    </svg>
    <div v-if="tip" class="tooltip" :style="{ left: tip.left + 'px', top: '8px' }">
      <div class="t">{{ tip.title }}</div>
      <div class="r" v-for="k in [...keys].reverse()" :key="k.key"><i class="dot" :style="{ background: k.color }"></i>{{ k.label }}<b>{{ tip.p[k.key] || 0 }}</b></div>
      <div class="r" style="border-top:1px solid var(--line);margin-top:4px;padding-top:4px">合计<b>{{ tip.total }} {{ unit }}</b></div>
    </div>
  </div>
</template>
