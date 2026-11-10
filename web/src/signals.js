export function signalValue(signal) {
  if (signal.status === 'error') return signal.reason
  if (signal.status !== 'available') {
    return {permission_denied:'未授权',unavailable:'暂不可用',disabled:'未开启'}[signal.status]
  }
  const value = signal.value
  if (value === true) return '是'
  if (value === false) return '否'
  if (Array.isArray(value)) {
    if(value.some(item=>item.package))return value.map(item=>item.package+' · '+({visible:'已采集签名',not_visible_or_absent:'未检出',permission_denied:'未授权'}[item.status]||item.status)).join('；')
    if(value.some(item=>item.vendor))return value.map(item=>[item.name,item.vendor].filter(Boolean).join(' · ')).join('；')
    return value.map(cell => {
      const parts = [cell.technology]
      if (cell.dbm != null) parts.push(cell.dbm + ' dBm')
      if (cell.ageMillis != null) parts.push(Math.floor(cell.ageMillis / 1000) + ' 秒前')
      if (cell.status === 'unsupported') parts.push('暂未解析')
      return parts.join(' · ')
    }).join('；')
  }
  if (typeof value === 'object' && value !== null) {
    if('memoryBytes' in value)return [value.brand,value.model,value.platform,value.machine,value.memoryBytes?((value.memoryBytes/1073741824).toFixed(2)+' GiB'):null,(value.configuredCores||value.availableCores)?((value.configuredCores||value.availableCores)+' 核'):null].filter(Boolean).join(' · ')
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
