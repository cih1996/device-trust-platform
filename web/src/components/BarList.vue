<script setup>
import { computed } from 'vue'
const props = defineProps({
  rows: { type: Array, required: true }, // [{ key, label, value, meta?, color?, level? }]
  empty: { type: String, default: '暂无数据' },
  clickable: { type: Boolean, default: false },
})
const emit = defineEmits(['select'])
const max = computed(() => Math.max(1, ...props.rows.map(r => r.value)))
</script>

<template>
  <div class="barlist">
    <div v-for="r in rows" :key="r.key" :class="['row', { click: clickable }]" @click="clickable && emit('select', r)">
      <span class="name" :title="r.label"><i v-if="r.level" :class="['dot', r.level]"></i><slot name="label" :row="r">{{ r.label }}</slot></span>
      <span class="val">{{ r.value.toLocaleString('zh-CN') }}</span>
      <span class="bar"><i :style="{ width: (r.value / max) * 100 + '%', background: r.color || 'var(--accent)' }"></i></span>
      <span v-if="r.meta" class="meta">{{ r.meta }}</span>
    </div>
    <div v-if="!rows.length" class="muted" style="padding:12px 0">{{ empty }}</div>
  </div>
</template>
