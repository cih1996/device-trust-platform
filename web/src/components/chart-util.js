import { ref, onMounted, onBeforeUnmount } from 'vue'

export function useWidth() {
  const el = ref(null)
  const width = ref(600)
  let observer
  onMounted(() => {
    observer = new ResizeObserver(([entry]) => { width.value = Math.max(200, Math.floor(entry.contentRect.width)) })
    observer.observe(el.value)
  })
  onBeforeUnmount(() => observer?.disconnect())
  return { el, width }
}

// Round axis maximum up to a clean step so ticks read 0 / 5 / 10 …
export function niceTicks(max, count = 4, integer = true) {
  if (!(max > 0)) return [0, 1]
  const raw = max / count
  const mag = 10 ** Math.floor(Math.log10(raw))
  let step = [1, 2, 2.5, 5, 10].map(m => m * mag).find(s => s >= raw)
  if (integer) step = Math.max(1, Math.ceil(step))
  const ticks = []
  for (let v = 0; v <= max + step * 0.001; v += step) ticks.push(Math.round(v * 100) / 100)
  if (ticks[ticks.length - 1] < max) ticks.push(ticks[ticks.length - 1] + step)
  return ticks
}

export function axisLabel(v) {
  return v >= 10000 ? (v / 10000).toFixed(v % 10000 ? 1 : 0) + '万' : v.toLocaleString('zh-CN')
}

export function bucketLabel(start, step) {
  const d = new Date(start)
  if (step === 'hour') return String(d.getHours()).padStart(2, '0') + ':00'
  return (d.getMonth() + 1) + '/' + d.getDate()
}

// Column with rounded data-end (top) and square baseline.
export function columnPath(x, y, w, h, r = 4) {
  if (h <= 0) return ''
  r = Math.min(r, h, w / 2)
  return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`
}
