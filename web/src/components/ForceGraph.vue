<script setup>
import { computed, ref } from 'vue'
import { useWidth } from './chart-util.js'

// Clusters are laid out one by one (force-directed) and packed in rows, so a
// studio of hundreds of phones never pushes the other clusters off screen.
const props = defineProps({
  nodes: { type: Array, required: true }, // [{ id, cluster, model, riskLevel, riskScore, lanIp, appName }]
  edges: { type: Array, required: true }, // [{ source, target, score, reasons: [{ kind, detail }] }]
  kinds: { type: Object, required: true }, // kind -> { label, color, dash }
})
const emit = defineEmits(['open'])
const { el, width } = useWidth()
const hover = ref(null)

function layoutCluster(ids, links) {
  const n = ids.length
  const index = new Map(ids.map((id, i) => [id, i]))
  // Deterministic start on a circle; same data always draws the same picture.
  const p = ids.map((_, i) => ({ x: Math.cos((2 * Math.PI * i) / n), y: Math.sin((2 * Math.PI * i) / n) }))
  const k = 1 / Math.sqrt(n)
  const springs = links.map(e => [index.get(e.source), index.get(e.target), e.score])
  for (let step = 0, t = 0.12; step < 220; step++, t *= 0.985) {
    const d = p.map(() => ({ x: 0, y: 0 }))
    for (let i = 0; i < n; i++) for (let j = i + 1; j < n; j++) {
      const dx = p[i].x - p[j].x, dy = p[i].y - p[j].y, dist = Math.max(Math.hypot(dx, dy), 1e-3)
      const f = (k * k) / dist / dist
      d[i].x += dx * f; d[i].y += dy * f; d[j].x -= dx * f; d[j].y -= dy * f
    }
    for (const [a, b, s] of springs) {
      const dx = p[a].x - p[b].x, dy = p[a].y - p[b].y, dist = Math.hypot(dx, dy)
      const f = (dist / k) * s
      d[a].x -= dx * f; d[a].y -= dy * f; d[b].x += dx * f; d[b].y += dy * f
    }
    for (let i = 0; i < n; i++) {
      d[i].x -= p[i].x * 0.6; d[i].y -= p[i].y * 0.6 // gravity keeps loose parts near the middle
      const len = Math.max(Math.hypot(d[i].x, d[i].y), 1e-9), m = Math.min(len, t)
      p[i].x += (d[i].x / len) * m; p[i].y += (d[i].y / len) * m
    }
  }
  const r = Math.max(...p.map(q => Math.hypot(q.x, q.y)), 1e-3)
  return p.map(q => ({ x: q.x / r, y: q.y / r }))
}

const scene = computed(() => {
  const w = Math.max(width.value, 280)
  const byCluster = new Map()
  for (const node of props.nodes) {
    if (!byCluster.has(node.cluster)) byCluster.set(node.cluster, [])
    byCluster.get(node.cluster).push(node)
  }
  const nodeAt = new Map()
  const groups = []
  let x = 0, y = 0, rowH = 0
  const gap = 24
  for (const [cluster, members] of byCluster) {
    const ids = members.map(n => n.id)
    const set = new Set(ids)
    const links = props.edges.filter(e => set.has(e.source) && set.has(e.target))
    const radius = Math.min(w / 2 - gap, 28 + 22 * Math.sqrt(members.length))
    const pts = layoutCluster(ids, links)
    const size = radius * 2 + gap
    if (x + size > w && x > 0) { x = 0; y += rowH; rowH = 0 }
    const cx = x + radius + gap / 2, cy = y + radius + gap / 2 + 14
    members.forEach((n, i) => nodeAt.set(n.id, { ...n, x: cx + pts[i].x * radius, y: cy + pts[i].y * radius }))
    groups.push({ cluster, size: members.length, x: cx, y: cy - radius - 8 })
    x += size
    rowH = Math.max(rowH, size + 14)
  }
  const lines = props.edges.filter(e => nodeAt.has(e.source) && nodeAt.has(e.target)).map(e => {
    const a = nodeAt.get(e.source), b = nodeAt.get(e.target), kind = props.kinds[e.reasons[0].kind]
    return { e, x1: a.x, y1: a.y, x2: b.x, y2: b.y, color: kind.color, dash: kind.dash, width: 1 + 2 * e.score }
  })
  return { w, h: y + rowH + 8, nodes: [...nodeAt.values()], lines, groups }
})

function show(event, kind, item) {
  const box = el.value.getBoundingClientRect()
  hover.value = { kind, item, x: event.clientX - box.left + 12, y: event.clientY - box.top + 12 }
}
const levelColor = { high: 'var(--high)', medium: 'var(--medium)', low: 'var(--low)' }
const levelText = { high: '高风险', medium: '中风险', low: '低风险' }
const nodeName = id => scene.value.nodes.find(n => n.id === id)?.model || id.slice(0, 8)
</script>

<template>
  <div ref="el" style="position:relative">
    <svg :width="scene.w" :height="scene.h" role="img" aria-label="设备关系图谱" style="display:block">
      <text v-for="g in scene.groups" :key="'g' + g.cluster" :x="g.x" :y="g.y" text-anchor="middle" class="axis" style="fill:var(--ink-2)">#{{ g.cluster }} · {{ g.size }} 台</text>
      <g v-for="(l, i) in scene.lines" :key="'l' + i">
        <line :x1="l.x1" :y1="l.y1" :x2="l.x2" :y2="l.y2" :stroke="l.color" :stroke-width="l.width" :stroke-dasharray="l.dash" stroke-opacity=".7" />
        <line :x1="l.x1" :y1="l.y1" :x2="l.x2" :y2="l.y2" stroke="transparent" stroke-width="10" style="cursor:help" @mousemove="show($event, 'edge', l.e)" @mouseleave="hover = null" />
      </g>
      <circle v-for="n in scene.nodes" :key="n.id" :cx="n.x" :cy="n.y" r="7" :fill="levelColor[n.riskLevel] || 'var(--muted)'" stroke="var(--surface)" stroke-width="2" style="cursor:pointer"
        @mousemove="show($event, 'node', n)" @mouseleave="hover = null" @click="emit('open', n)" />
    </svg>
    <div v-if="hover" class="tooltip" :style="{ left: hover.x + 'px', top: hover.y + 'px' }">
      <template v-if="hover.kind === 'node'">
        <div class="t">{{ hover.item.model }}</div>
        <div class="r">应用<b>{{ hover.item.appName }}</b></div>
        <div class="r">风险<b>{{ levelText[hover.item.riskLevel] || '未评分' }} {{ hover.item.riskScore }}</b></div>
        <div v-if="hover.item.lanIp" class="r">内网 IP<b>{{ hover.item.lanIp }}</b></div>
        <div class="r">团伙<b>#{{ hover.item.cluster }}</b></div>
      </template>
      <template v-else>
        <div class="t">{{ nodeName(hover.item.source) }} ↔ {{ nodeName(hover.item.target) }}</div>
        <div class="r">关联系数<b>{{ hover.item.score.toFixed(2) }}</b></div>
        <div v-for="r in hover.item.reasons" :key="r.kind" class="r"><i :style="{ width: '10px', height: '2px', background: kinds[r.kind].color }"></i>{{ r.detail }}</div>
      </template>
    </div>
  </div>
</template>
