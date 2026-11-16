<script setup>
import { levelOf, levelNames } from '../format.js'
defineProps({ score: { type: Number, default: 0 }, label: { type: Boolean, default: false } })
</script>
<template>
  <span class="score-cell" :title="'风险评分 ' + score + ' / 100 · ' + levelNames[levelOf(score)]">
    <strong>{{ score }}</strong>
    <span class="meter"><i :class="'bg-' + levelOf(score)" :style="{ width: Math.max(2, Math.min(100, score)) + '%' }"></i></span>
    <span v-if="label" :class="['level', levelOf(score)]">{{ levelNames[levelOf(score)] }}</span>
  </span>
</template>
