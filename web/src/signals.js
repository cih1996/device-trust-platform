export function signalValue(signal) {
  if (signal.status === 'error') return signal.reason
  if (signal.status !== 'available') {
    return {permission_denied:'未授权',unavailable:'暂不可用',disabled:'未开启'}[signal.status]
  }
  const value = signal.value
  if (value === true) return '是'
  if (value === false) return '否'
  if (Array.isArray(value)) {
    return value.map(cell => {
      const parts = [cell.technology]
      if (cell.dbm != null) parts.push(cell.dbm + ' dBm')
      if (cell.ageMillis != null) parts.push(Math.floor(cell.ageMillis / 1000) + ' 秒前')
      if (cell.status === 'unsupported') parts.push('暂未解析')
      return parts.join(' · ')
    }).join('；')
  }
  if (typeof value === 'object' && value !== null) {
    if ('latitude' in value && 'longitude' in value) {
      const parts = [value.latitude + ', ' + value.longitude]
      if (value.accuracyMeters != null) parts.push('±' + value.accuracyMeters + ' 米')
      if (value.ageMillis != null) parts.push(Math.floor(value.ageMillis / 1000) + ' 秒前')
      if (value.mock) parts.push('模拟位置')
      return parts.join(' · ')
    }
    return JSON.stringify(value)
  }
  return String(value)
}
