<script setup>
import { computed } from 'vue'
import { levelOf } from '../format.js'
const props = defineProps({ score: { type: Number, default: 0 }, size: { type: Number, default: 72 } })
const r = 30, C = 2 * Math.PI * r
const level = computed(() => levelOf(props.score))
const dash = computed(() => `${(Math.max(0, Math.min(100, props.score)) / 100) * C} ${C}`)
</script>
<template>
  <svg :width="size" :height="size" viewBox="0 0 72 72" role="img" :aria-label="'风险评分 ' + score">
    <circle cx="36" cy="36" :r="r" fill="none" :stroke="`var(--${level}-weak)`" stroke-width="7" />
    <circle cx="36" cy="36" :r="r" fill="none" :stroke="`var(--${level})`" stroke-width="7" stroke-linecap="round" :stroke-dasharray="dash" transform="rotate(-90 36 36)" />
    <text x="36" y="42" text-anchor="middle" style="font-size:19px;font-weight:600;fill:var(--ink)">{{ score }}</text>
  </svg>
</template>
