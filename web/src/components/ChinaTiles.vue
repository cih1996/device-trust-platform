<script setup>
import { computed, ref } from 'vue'

// Tile-grid map of China: one equal square per province, laid out roughly by
// geography. Equal tiles keep small, dense provinces (上海, 北京) readable.
const props = defineProps({
  values: { type: Object, required: true }, // short name -> { count, high, raw }
  metric: { type: String, default: 'count' }, // count | highShare
  selected: { type: String, default: '' }, // short name
})
const emit = defineEmits(['select'])
const hover = ref(null)

const tiles = [
  ['黑龙江', 8, 0],
  ['内蒙古', 4, 1], ['北京', 5, 1], ['辽宁', 7, 1], ['吉林', 8, 1],
  ['新疆', 0, 2], ['甘肃', 2, 2], ['宁夏', 3, 2], ['山西', 4, 2], ['河北', 5, 2], ['天津', 6, 2],
  ['青海', 1, 3], ['陕西', 3, 3], ['河南', 4, 3], ['山东', 5, 3],
  ['西藏', 1, 4], ['四川', 2, 4], ['重庆', 3, 4], ['湖北', 4, 4], ['安徽', 5, 4], ['江苏', 6, 4],
  ['云南', 2, 5], ['贵州', 3, 5], ['湖南', 4, 5], ['江西', 5, 5], ['浙江', 6, 5], ['上海', 7, 5],
  ['广西', 3, 6], ['广东', 4, 6], ['福建', 5, 6], ['台湾', 7, 6],
  ['海南', 3, 7], ['澳门', 4, 7], ['香港', 5, 7],
]
const size = 46, gap = 4

// Five sequential steps of one hue; the empty tile stays neutral.
const steps = [16, 32, 52, 74, 100]
const scale = computed(() => {
  const vals = Object.values(props.values).map(v => (props.metric === 'count' ? v.count : v.count ? v.high / v.count : 0))
  return Math.max(...vals, props.metric === 'count' ? 1 : 0.01)
})
function cell(name) {
  const v = props.values[name]
  if (!v || !v.count) return { fill: 'var(--surface-2)', ink: 'var(--muted)', step: -1 }
  const x = props.metric === 'count' ? v.count / scale.value : v.high / v.count / scale.value
  const step = x <= 0 ? -1 : Math.min(4, Math.floor(x * 5 - 1e-9))
  if (step < 0) return { fill: 'var(--surface-2)', ink: 'var(--ink-2)', step }
  const hue = props.metric === 'count' ? 'var(--accent)' : 'var(--high)'
  return { fill: `color-mix(in srgb, ${hue} ${steps[step]}%, var(--surface))`, ink: step >= 3 ? '#fff' : 'var(--ink)', step }
}
const legend = computed(() => steps.map((p, i) => ({
  fill: `color-mix(in srgb, ${props.metric === 'count' ? 'var(--accent)' : 'var(--high)'} ${p}%, var(--surface))`,
  text: props.metric === 'count' ? Math.ceil((scale.value * (i + 1)) / 5) : Math.round((scale.value * (i + 1) * 100) / 5) + '%',
})))
function show(event, name) {
  const box = event.currentTarget.ownerSVGElement.getBoundingClientRect()
  hover.value = { name, x: event.clientX - box.left + 12, y: event.clientY - box.top + 12 }
}
const pct = (a, b) => (b ? Math.round((a / b) * 100) + '%' : '—')
</script>

<template>
  <div style="position:relative">
    <svg :viewBox="`0 0 ${9 * (size + gap)} ${8 * (size + gap)}`" style="display:block;width:100%;max-width:470px;margin:0 auto" role="img" aria-label="省份分布图">
      <g v-for="[name, c, r] in tiles" :key="name" :transform="`translate(${c * (size + gap)},${r * (size + gap)})`"
        :style="{ cursor: values[name]?.count ? 'pointer' : 'default' }"
        @mousemove="show($event, name)" @mouseleave="hover = null" @click="values[name]?.count && emit('select', name)">
        <rect :width="size" :height="size" rx="6" :fill="cell(name).fill" :stroke="selected === name ? 'var(--ink)' : 'none'" stroke-width="2" />
        <text :x="size / 2" :y="size / 2 - 3" text-anchor="middle" font-size="11" :fill="cell(name).ink">{{ name.length > 3 ? name.slice(0, 2) : name }}</text>
        <text :x="size / 2" :y="size / 2 + 11" text-anchor="middle" font-size="10" :fill="cell(name).ink" style="font-variant-numeric:tabular-nums">{{ values[name]?.count ? (metric === 'count' ? values[name].count : pct(values[name].high, values[name].count)) : '' }}</text>
      </g>
    </svg>
    <div class="legend" style="justify-content:center;margin-top:8px">
      <span><i style="background:var(--surface-2)"></i>无</span>
      <span v-for="(l, i) in legend" :key="i"><i :style="{ background: l.fill }"></i>≤ {{ l.text }}</span>
    </div>
    <div v-if="hover" class="tooltip" :style="{ left: hover.x + 'px', top: hover.y + 'px' }">
      <div class="t">{{ values[hover.name]?.raw || hover.name }}</div>
      <div class="r">设备<b>{{ values[hover.name]?.count || 0 }}</b></div>
      <div class="r">高风险<b>{{ values[hover.name]?.high || 0 }}（{{ pct(values[hover.name]?.high || 0, values[hover.name]?.count || 0) }}）</b></div>
    </div>
  </div>
</template>
