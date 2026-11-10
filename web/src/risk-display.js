export const sceneNames = {registration:'注册保护',marketing:'营销保护',payment:'支付保护',general:'其他事件'}
export const actionNames = {observe:'观察',verify:'建议核验',hold:'建议暂缓'}
export const reviewNames = {fraud:'确认异常',legitimate:'确认正常',unknown:'待确认'}
export const riskReasons = {
  debugger:'正在调试',emulator_hint:'模拟环境线索',vpn:'VPN 连接',package_mismatch:'应用不匹配',
  multiple_accounts_24h:'设备关联多个账号',frequent_events_1m:'设备操作频繁',root_indicators:'多项系统修改线索',
  account_device_changes:'账号频繁切换设备',registration_device_cluster:'同设备集中注册',registration_ip_cluster:'同网络集中注册',
  campaign_device_accounts:'同设备多账号参与活动',campaign_shared_recipient:'多个账号使用同一领取对象',
  campaign_repeated_claims:'短时间重复领取',new_account_campaign_cluster:'新账号集中参与活动',
  payment_attempt_velocity:'支付尝试频繁',payment_method_rotation:'频繁更换支付工具',payment_shared_method:'多个账号共用支付工具',payment_failure_history:'近期支付失败较多',payment_chargeback_history:'账号存在近期拒付记录'
}
export const fieldNames = {'clientIp':'用户网络','context.campaignId':'活动','context.recipientId':'领取对象','context.paymentMethodId':'支付工具','context.orderId':'订单'}
export const outcomeNames = {failed:'支付失败',succeeded:'支付成功',chargeback:'拒付记录'}
