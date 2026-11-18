<script setup>
import { computed, ref } from 'vue'

// Weekday × hour activity grid, one sequential hue.
const props = defineProps({ grid: { type: Array, required: true } }) // 7 × 24 counts, Monday first
const days = ['周一', '周二', '周三', '周四', '周五', '周六', '周日']
const hover = ref(null)
const max = computed(() => Math.max(1, ...props.grid.flat()))
const fill = n => (n ? `color-mix(in srgb, var(--accent) ${Math.round(12 + 88 * Math.sqrt(n / max.value))}%, var(--surface))` : 'var(--surface-2)')
const total = computed(() => props.grid.flat().reduce((a, b) => a + b, 0))
const night = computed(() => props.grid.reduce((s, row) => s + row.slice(0, 6).reduce((a, b) => a + b, 0), 0))
function show(event, d, h) {
  const box = event.currentTarget.closest('.heat').getBoundingClientRect()
  hover.value = { d, h, x: event.clientX - box.left + 12, y: event.clientY - box.top + 12 }
}
</script>

<template>
  <div class="heat" style="position:relative">
    <div class="grid7">
      <template v-for="(row, d) in grid" :key="d">
        <span class="day">{{ days[d] }}</span>
        <span v-for="(n, h) in row" :key="h" class="c" :style="{ background: fill(n) }" @mousemove="show($event, d, h)" @mouseleave="hover = null"></span>
      </template>
      <span></span>
      <span v-for="h in 24" :key="'h' + h" class="hour">{{ (h - 1) % 6 === 0 ? h - 1 : '' }}</span>
    </div>
    <p class="muted" style="font-size:12px;margin:10px 0 0">共 {{ total.toLocaleString('zh-CN') }} 次上报与事件，凌晨 0–6 点占 {{ total ? Math.round((night / total) * 100) : 0 }}%。挂机设备常在深夜仍保持规律活跃。</p>
    <div v-if="hover" class="tooltip" :style="{ left: hover.x + 'px', top: hover.y + 'px' }">
      <div class="t">{{ days[hover.d] }} {{ hover.h }}:00–{{ hover.h + 1 }}:00</div>
      <div class="r">上报与事件<b>{{ grid[hover.d][hover.h] }}</b></div>
    </div>
  </div>
</template>

<style scoped>
.grid7 { display: grid; grid-template-columns: 34px repeat(24, minmax(0, 1fr)); gap: 2px; align-items: center; }
.c { aspect-ratio: 1; border-radius: 3px; min-height: 8px; }
.day, .hour { font-size: 11px; color: var(--muted); }
.hour { text-align: left; }
</style>
