<script setup>
import { computed, ref } from 'vue'
import { useWidth, niceTicks, axisLabel, bucketLabel } from './chart-util.js'

const props = defineProps({
  points: { type: Array, required: true },
  field: { type: String, required: true },
  label: { type: String, default: '' },
  color: { type: String, default: 'var(--accent)' },
  step: { type: String, default: 'hour' },
  height: { type: Number, default: 160 },
  max: { type: Number, default: 0 }, // fixed y max, e.g. 100 for scores
  unit: { type: String, default: '' },
  emptyAs: { type: String, default: '' }, // points with no events are gaps, not zero
})
const { el, width } = useWidth()
const hover = ref(-1)
const pad = { top: 10, right: 12, bottom: 24, left: 36 }

const layout = computed(() => {
  const n = props.points.length
  const values = props.points.map(p => (props.emptyAs && !p[props.emptyAs] ? null : p[props.field] ?? 0))
  const ticks = props.max ? [0, props.max / 4, props.max / 2, (props.max * 3) / 4, props.max] : niceTicks(Math.max(0, ...values.filter(v => v != null)))
  const top = ticks[ticks.length - 1] || 1
  const plotW = width.value - pad.left - pad.right
  const plotH = props.height - pad.top - pad.bottom
  const x = i => pad.left + (n <= 1 ? plotW / 2 : (plotW * i) / (n - 1))
  const y = v => pad.top + plotH - (v / top) * plotH
  let line = '', area = '', run = []
  const flush = () => {
    if (!run.length) return
    line += run.map(([i, v], j) => (j ? 'L' : 'M') + x(i) + ',' + y(v)).join('')
    area += 'M' + x(run[0][0]) + ',' + y(0) + run.map(([i, v]) => 'L' + x(i) + ',' + y(v)).join('') + 'L' + x(run[run.length - 1][0]) + ',' + y(0) + 'Z'
    run = []
  }
  values.forEach((v, i) => (v == null ? flush() : run.push([i, v])))
  flush()
  const labelEvery = Math.ceil(n / Math.max(1, Math.floor(plotW / 48)))
  const dots = values.map((v, i) => ({ i, x: x(i), y: v == null ? null : y(v), v, label: i % labelEvery === 0 ? bucketLabel(props.points[i].start, props.step) : '' }))
  return { ticks: ticks.map(t => ({ v: t, y: y(t) })), line, area, dots, plotH, plotW, n }
})
function move(event) {
  const { left } = el.value.getBoundingClientRect()
  const px = event.clientX - left - pad.left
  const n = layout.value.n
  hover.value = Math.max(0, Math.min(n - 1, Math.round((px / layout.value.plotW) * (n - 1))))
}
const tip = computed(() => {
  if (hover.value < 0) return null
  const d = layout.value.dots[hover.value]
  const p = props.points[hover.value]
  const date = new Date(p.start)
  const title = props.step === 'hour' ? String(date.getHours()).padStart(2, '0') + ':00' : date.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric', weekday: 'short' })
  return { d, title, left: Math.min(d.x + 12, width.value - 150) }
})
</script>

<template>
  <div class="chart" ref="el" @mousemove="move" @mouseleave="hover = -1">
    <svg :width="width" :height="height" role="img" :aria-label="label + '折线图'">
      <template v-for="t in layout.ticks" :key="t.v">
        <line :class="t.v === 0 ? 'baseline' : 'gridline'" :x1="pad.left" :x2="width - pad.right" :y1="t.y" :y2="t.y" />
        <text class="axis" :x="pad.left - 6" :y="t.y + 4" text-anchor="end">{{ axisLabel(t.v) }}</text>
      </template>
      <path :d="layout.area" :fill="color" opacity=".1" />
      <path :d="layout.line" :stroke="color" stroke-width="2" fill="none" stroke-linejoin="round" stroke-linecap="round" />
      <template v-for="d in layout.dots" :key="d.i">
        <text v-if="d.label" class="axis" :x="d.x" :y="height - 6" text-anchor="middle">{{ d.label }}</text>
      </template>
      <template v-if="tip">
        <line :x1="tip.d.x" :x2="tip.d.x" :y1="pad.top" :y2="pad.top + layout.plotH" stroke="var(--line-strong)" />
        <circle v-if="tip.d.y != null" :cx="tip.d.x" :cy="tip.d.y" r="4.5" :fill="color" stroke="var(--surface)" stroke-width="2" />
      </template>
    </svg>
    <div v-if="tip" class="tooltip" :style="{ left: tip.left + 'px', top: '8px' }">
      <div class="t">{{ tip.title }}</div>
      <div class="r"><i class="dot" :style="{ background: color }"></i>{{ label }}<b>{{ tip.d.v == null ? '无数据' : tip.d.v + unit }}</b></div>
    </div>
  </div>
</template>
