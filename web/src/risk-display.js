export const sceneNames = {registration:'注册保护',marketing:'营销保护',payment:'支付保护',general:'其他事件'}
export const actionNames = {observe:'观察',verify:'建议核验',hold:'建议暂缓'}
export const reviewNames = {fraud:'确认异常',legitimate:'确认正常',unknown:'待确认'}
export const riskReasons = {
  build_native_mismatch:'机型参数矛盾',memory_source_mismatch:'内存读数矛盾',cpu_core_mismatch:'CPU 核心数矛盾',device_spec_mismatch:'机型规格不符',system_package_signer_mismatch:'系统应用签名异常',mock_location:'系统标记模拟位置',cell_gps_mismatch:'基站与定位矛盾',ip_gps_region_mismatch:'网络与定位区域不一致',
  debugger:'正在调试',emulator_hint:'模拟环境线索',vpn:'VPN 连接',package_mismatch:'应用不匹配',
  multiple_accounts_24h:'设备关联多个账号',frequent_events_1m:'设备操作频繁',root_indicators:'多项系统修改线索',
  account_device_changes:'账号频繁切换设备',registration_device_cluster:'同设备集中注册',registration_ip_cluster:'同网络集中注册',
  campaign_device_accounts:'同设备多账号参与活动',campaign_shared_recipient:'多个账号使用同一领取对象',
  campaign_repeated_claims:'短时间重复领取',new_account_campaign_cluster:'新账号集中参与活动',
  native_debugger:'底层调试器附加',hook_framework:'Hook 框架痕迹',hook_threads:'Hook 注入线程',emulator_traits:'模拟器特征',x86_runtime:'x86 运行环境',missing_motion_sensor:'缺少加速度传感器',app_debuggable:'应用为可调试版本',http_proxy:'HTTP 代理',virtual_container:'多开 / 虚拟容器',secondary_user:'分身或多用户空间',adb_enabled:'USB 调试已开启',
  payment_attempt_velocity:'支付尝试频繁',payment_method_rotation:'频繁更换支付工具',payment_shared_method:'多个账号共用支付工具',payment_failure_history:'近期支付失败较多',payment_chargeback_history:'账号存在近期拒付记录'
}
export const fieldNames = {'clientIp':'用户网络','context.campaignId':'活动','context.recipientId':'领取对象','context.paymentMethodId':'支付工具','context.orderId':'订单'}
export const outcomeNames = {failed:'支付失败',succeeded:'支付成功',chargeback:'拒付记录'}
export const statusNames = {observed:'仅观察',pending:'待处理',approved:'已放行',rejected:'已拒绝',executed:'业务已完成',failed:'执行失败'}
export const sourceNames = {client:'设备环境',resource:'环境验真',business:'业务行为',outcome:'交易结果'}
export const checkNames = {hardware_crosscheck:'硬件采集',build_native_mismatch:'机型参数矛盾',build_native_crosscheck:'机型交叉比对',memory_source_mismatch:'内存读数矛盾',memory_source_crosscheck:'内存交叉比对',cpu_core_mismatch:'CPU 核心数矛盾',native_resource_errors:'底层采集',device_baseline:'机型基线',device_spec_mismatch:'机型规格矛盾',firmware_baseline:'固件覆盖',package_baseline:'系统应用覆盖',system_package_coverage:'系统应用覆盖',system_package_signer_mismatch:'系统应用签名异常',system_package_signer:'系统应用签名',ip_region:'IP 归属',gps_coverage:'定位覆盖',mock_location:'模拟定位',cell_coverage:'基站采集',cell_location:'基站坐标覆盖',cell_gps_mismatch:'基站与定位矛盾',cell_gps_crosscheck:'基站与定位比对',ip_gps_region:'网络区域比对',ip_gps_region_mismatch:'网络与定位区域不一致'}
export const checkStates = {matched:'一致',conflict:'矛盾',unknown:'信息不足',stale:'暂不可比',error:'采集出错'}
export const reasonName = code => riskReasons[code] || checkNames[code] || code
