<script setup>
import { computed, ref } from 'vue'
const props = defineProps({
  segments: { type: Array, required: true }, // [{ key, label, value, color }]
  centerLabel: { type: String, default: '' },
  size: { type: Number, default: 148 },
})
const emit = defineEmits(['select'])
const hover = ref('')
const r = 54, stroke = 14, C = 2 * Math.PI * r
const total = computed(() => props.segments.reduce((s, x) => s + x.value, 0))
const arcs = computed(() => {
  let offset = 0
  const visible = props.segments.filter(s => s.value > 0)
  const gap = visible.length > 1 ? 2 : 0 // surface gap between arcs
  return visible.map(s => {
    const len = (s.value / total.value) * C
    const arc = { ...s, dash: `${Math.max(0, len - gap)} ${C}`, offset: -offset }
    offset += len
    return arc
  })
})
const focus = computed(() => props.segments.find(s => s.key === hover.value))
const pct = v => (total.value ? Math.round((v / total.value) * 100) : 0) + '%'
</script>

<template>
  <div class="donut" style="display:flex;gap:20px;align-items:center;flex-wrap:wrap">
    <svg :width="size" :height="size" viewBox="0 0 148 148" role="img" :aria-label="centerLabel + '占比'" style="flex:none">
      <circle cx="74" cy="74" :r="r" fill="none" stroke="var(--surface-2)" :stroke-width="stroke" />
      <circle v-for="a in arcs" :key="a.key" cx="74" cy="74" :r="r" fill="none" :stroke="a.color" :stroke-width="hover === a.key ? stroke + 4 : stroke"
        :stroke-dasharray="a.dash" :stroke-dashoffset="a.offset" transform="rotate(-90 74 74)" style="cursor:pointer;transition:stroke-width .15s"
        @mouseenter="hover = a.key" @mouseleave="hover = ''" @click="emit('select', a.key)" />
      <text x="74" y="72" text-anchor="middle" style="font-size:24px;font-weight:600;fill:var(--ink)">{{ focus ? focus.value : total }}</text>
      <text x="74" y="92" text-anchor="middle" style="font-size:12px;fill:var(--muted)">{{ focus ? focus.label + ' · ' + pct(focus.value) : centerLabel }}</text>
    </svg>
    <div class="barlist" style="flex:1;min-width:140px">
      <div v-for="s in segments" :key="s.key" class="row click" @mouseenter="hover = s.key" @mouseleave="hover = ''" @click="emit('select', s.key)">
        <span class="name"><i class="dot" :style="{ background: s.color }"></i>{{ s.label }}</span>
        <span class="val">{{ s.value }} <small>{{ pct(s.value) }}</small></span>
      </div>
    </div>
  </div>
</template>
