<script setup>
import { computed, ref } from 'vue'

// Buckets of one dimension: count, a bar split by risk level, high-risk share.
const props = defineProps({
  rows: { type: Array, required: true }, // [{ value, label, count, low, medium, high }]
  selected: { type: String, default: null },
  limit: { type: Number, default: 8 },
})
const emit = defineEmits(['select'])
const open = ref(false)
const shown = computed(() => (open.value ? props.rows : props.rows.slice(0, props.limit)))
const max = computed(() => Math.max(1, ...props.rows.map(r => r.count)))
const parts = r => [['low', r.low], ['medium', r.medium], ['high', r.high]].filter(([, n]) => n > 0)
</script>

<template>
  <div class="buckets">
    <div v-for="r in shown" :key="r.value" :class="['bucket', { on: selected === r.value, dim: selected != null && selected !== r.value }]"
      :title="`${r.label}：${r.count}（高 ${r.high} · 中 ${r.medium} · 低 ${r.low}）`" @click="emit('select', r.value)">
      <span class="name">{{ r.label }}</span>
      <span class="val">{{ r.count.toLocaleString('zh-CN') }}</span>
      <span class="meta" :class="{ bad: r.high / r.count >= 0.3 }">{{ r.count ? Math.round((r.high / r.count) * 100) : 0 }}%</span>
      <span class="track"><span class="fill" :style="{ width: (r.count / max) * 100 + '%' }"><i v-for="[k, n] in parts(r)" :key="k" :style="{ flexGrow: n, background: `var(--${k})` }"></i></span></span>
    </div>
    <div v-if="!rows.length" class="muted" style="padding:12px 0">暂无数据</div>
    <button v-if="rows.length > limit" class="link-btn" @click="open = !open">{{ open ? '收起' : `展开全部 ${rows.length} 项` }}</button>
  </div>
</template>

<style scoped>
.bucket { display: grid; grid-template-columns: minmax(0, 1fr) auto 44px; column-gap: 10px; row-gap: 4px; padding: 6px 8px; margin: 0 -8px; border-radius: 6px; cursor: pointer; font-size: 13px; }
.bucket:hover { background: var(--surface-hover); }
.bucket.on { background: var(--accent-weak); }
.bucket.dim { opacity: .55; }
.name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink); }
.val { font-variant-numeric: tabular-nums; color: var(--ink); text-align: right; }
.meta { font-variant-numeric: tabular-nums; color: var(--muted); text-align: right; font-size: 12px; }
.meta.bad { color: var(--high-ink); font-weight: 600; }
.track { grid-column: 1 / -1; height: 6px; }
.fill { display: flex; gap: 2px; height: 100%; min-width: 4px; }
.fill i { display: block; height: 100%; border-radius: 3px; min-width: 2px; }
.link-btn { border: 0; background: none; color: var(--accent-ink); padding: 6px 0 0; font-size: 13px; cursor: pointer; }
</style>
