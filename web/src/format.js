export const levelNames = { low: '低风险', medium: '中风险', high: '高风险' }
export const levelOf = score => (score >= 70 ? 'high' : score >= 30 ? 'medium' : 'low')

export function dateTime(value) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
export function fullTime(value) {
  return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
}
export function relative(value) {
  if (!value) return '—'
  const seconds = Math.round((Date.now() - new Date(value).getTime()) / 1000)
  if (seconds < 60) return '刚刚'
  if (seconds < 3600) return Math.floor(seconds / 60) + ' 分钟前'
  if (seconds < 86400) return Math.floor(seconds / 3600) + ' 小时前'
  if (seconds < 86400 * 30) return Math.floor(seconds / 86400) + ' 天前'
  return new Date(value).toLocaleDateString('zh-CN')
}
export function compact(n) {
  n = Number(n) || 0
  if (Math.abs(n) >= 1e8) return (n / 1e8).toFixed(1).replace(/\.0$/, '') + '亿'
  if (Math.abs(n) >= 1e4) return (n / 1e4).toFixed(1).replace(/\.0$/, '') + '万'
  return n.toLocaleString('zh-CN')
}
export function region(r) {
  return r ? [r.country === '中国' ? '' : r.country, r.province, r.city].filter(Boolean).join(' ') || r.country : ''
}
export function shortId(id) {
  return id && id.length > 16 ? id.slice(0, 8) + '…' + id.slice(-4) : id || ''
}
export function gib(value) {
  return typeof value === 'number' && value >= 0 ? (value / 1073741824).toFixed(2) + ' GiB' : '未提供'
}
