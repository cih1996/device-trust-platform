<script setup>
import { computed } from 'vue'
import { checkNames, checkStates } from '../risk-display.js'
import { region } from '../format.js'
const props = defineProps({ assessment: { type: Object, default: null } })
const order = { conflict: 0, error: 1, unknown: 2, stale: 3, matched: 4 }
const checks = computed(() => [...(props.assessment?.checks || [])].sort((a, b) => order[a.state] - order[b.state]))
const cls = { matched: 'good', conflict: 'bad', error: 'warn', unknown: '', stale: '' }
</script>
<template>
  <div v-if="assessment">
    <p v-if="assessment.ip" class="muted" style="margin:0 0 8px;font-size:13px">IP 归属：{{ region(assessment.ip) }} {{ assessment.ip.isp }}</p>
    <details v-for="(c, i) in checks" :key="i" class="factor" style="display:block">
      <summary style="display:flex;gap:8px;align-items:center;cursor:pointer">
        <span :class="['tag', cls[c.state]]">{{ checkStates[c.state] || c.state }}</span>{{ checkNames[c.code] || c.code }}
        <span v-if="c.points" class="pts" style="margin-left:auto">+{{ c.points }}</span>
      </summary>
      <p style="margin:6px 0 0;font-size:13px;color:var(--ink-2);overflow-wrap:anywhere">{{ c.detail || '无补充说明' }}</p>
      <small v-if="c.source">依据：{{ c.source }}</small>
    </details>
    <p v-if="!checks.length" class="muted">没有比对项</p>
  </div>
  <p v-else class="muted">本次上报没有环境验真结果</p>
</template>
