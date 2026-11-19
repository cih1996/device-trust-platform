// Rule and check names are shared with the platform API (Go embeds the same file).
import ruleNames from '../../internal/platform/rulenames.json'
export const sceneNames = {registration:'注册保护',marketing:'营销保护',payment:'支付保护',general:'其他事件'}
export const actionNames = {observe:'观察',verify:'建议核验',hold:'建议暂缓'}
export const reviewNames = {fraud:'确认异常',legitimate:'确认正常',unknown:'待确认'}
export const riskReasons = ruleNames.reasons
export const fieldNames = {'clientIp':'用户网络','context.campaignId':'活动','context.recipientId':'领取对象','context.paymentMethodId':'支付工具','context.orderId':'订单'}
export const outcomeNames = {failed:'支付失败',succeeded:'支付成功',chargeback:'拒付记录'}
export const statusNames = {observed:'仅观察',pending:'待处理',approved:'已放行',rejected:'已拒绝',executed:'业务已完成',failed:'执行失败'}
export const sourceNames = {client:'设备环境',resource:'环境验真',recovery:'良好使用',business:'业务行为',outcome:'交易结果'}
export const checkNames = ruleNames.checks
export const checkStates = {matched:'一致',conflict:'矛盾',unknown:'信息不足',stale:'暂不可比',error:'采集出错'}
export const reasonName = code => riskReasons[code] || checkNames[code] || code
export const signalNames = {behavior:'使用行为统计',drmId:'Widevine 设备 ID（摘要）',lan:'局域网',lanProbe:'局域网探测',accessibility:'无障碍状态',accessibilityServices:'已开启无障碍服务',activeInterface:'承载网卡',adbEnabled:'USB 调试',androidId:'ANDROID_ID',appDebuggable:'应用可调试',appSandbox:'应用沙箱',application:'应用与签名',attestation:'硬件证明',automation:'自动化与点击来源',battery:'电池',bluetoothMac:'蓝牙 MAC',bssid:'Wi-Fi BSSID',buildFingerprint:'系统指纹',cell:'基站',debugger:'调试器',developerOptions:'开发者选项',display:'显示',emulatorHint:'模拟器线索',expectedPackage:'包名校验',expectedSigner:'签名校验',httpProxy:'HTTP 代理',installedApps:'应用列表',integrityNative:'原生层完整性',javaHooks:'Java Hook',location:'定位',manufacturer:'制造商',memory:'内存',model:'型号',networkRoaming:'漫游',networkSnapshot:'网络快照',operator:'注册网络',osInfo:'系统信息',resourceJava:'Java 层机型参数',resourceNative:'原生层机型参数',riskApps:'风险应用',rootBeerMagiskBinary:'Magisk 文件',rootBeerNativeSu:'原生 su 检查',rootBeerSuBinary:'su 文件',rootBeerTestKeys:'测试签名系统',rootBeerVersion:'Root 检测版本',routerMac:'路由器 MAC',sdkInt:'API 版本',securityPatch:'安全补丁',sensors:'传感器',simOperator:'SIM 运营商',storage:'存储',systemPackages:'厂商系统应用',transport:'网络类型',vpn:'VPN',wifiRssi:'Wi-Fi 信号'}

export const signed = n => (n > 0 ? '+' + n : String(n))
