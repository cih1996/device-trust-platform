# DeviceTrust 0.6 接入与验证

首次打开 App，在用户同意采集后调用 DeviceTrustClient.initialize，先收集参数，再从业务服务取得启动挑战、完成密钥登记和加密报告上报，返回 deviceId。此过程不需要账号登录。后续启动复用同一应用的 Keystore 密钥与 deviceId，同时更新设备档案。清除数据、卸载或密钥更换可能产生新 ID；它不是永久物理硬件编号，也不是业务授权凭据。

## Android

必需 sdk-0.6.0.aar、sdk-secure-0.6.0.aar；示例 MainActivity.java 包含同意入口和后台线程调用。导入 com.device.trust.secure.DeviceTrustClient。

```java
// 在用户同意后，由宿主后台线程执行。
DeviceTrustClient.Config config = new DeviceTrustClient.Config(
    appId, businessUrl, new String[]{apiSpkiPin});
JSONObject result = DeviceTrustClient.initialize(context, config, true, null);
String deviceId = result.getString("deviceId");
// 后续可读取 DeviceTrustClient.deviceId(context, appId)。
```

businessUrl 为业务后端前缀。体验为 https://devicetrust-sdk-api.lingzidev.com/demo。appId 是公开标识；应用密钥只能留在业务后端。pins 需覆盖业务挑战服务和上报服务的实际证书公钥。初始化同步执行，不支持主线程；错误直接向宿主抛出。consent=false 时在采集前抛错。选配环境模块可通过 ExtraCollector 加入报告。

## 启动挑战接口

宿主业务后端实现 POST {businessUrl}/device/prepare，接收 appId、installationId、keyId。参考 internal/platform/startup.go：校验应用范围，生成 guest 观察身份和唯一启动事件号，计算启动请求摘要，通过服务端应用密钥请求 /v1/challenges。类型为 device_init / device_refresh。返回 {appId, platformUrl, challenge}，挑战响应完整保留。对公开入口配置流量限制；不能将其视为登录认证。

启动流程只登记和上报，不创建业务事件或领取执行授权。注册账号、领券、支付仍由已认证业务服务创建各自挑战，绑定账号、操作及 requestHash，再经 businessguard 校验授权，业务数据库按 operationId 幂等执行。deviceId 和启动报告不能替代此关口。单独复制 APK 示例不等于完成业务服务接入。

## 后台

设备档案支持搜索、分页，以及档案概览、机型与系统、应用与签名、网络与定位、关联与轨迹、原始数据六个分区。展示登记时间、稳定 ID、密钥状态、参数比对、账号与业务事件、最近上报和网络变化；可导出当前档案。旧版未采集的新字段需要更新 SDK 并重新上报。

新增采集包含系统版本、运行内存与可用量、数据分区容量、显示区域与密度、电池、传感器清单、宿主版本与 APK 签名。系统应用仅查询指定厂商包名，不枚举全部用户安装应用。Android 权限与系统可见性限制导致缺失时，保留实际状态。

## 本次验证

2026-11-11：两路 MI 8 SE / Android 10 / ARM64 受测连接，发布 APK 0.6.0 测试均返回 registered_before_login、same_device_id、consentBlocked=true；启动测试配置不含登录密码。每次报告包含 34 项采集定义（不是全部可用），服务端加入 expectedPackage 后共 35 项；实际记录 38 个传感器。设备档案实际显示 Snapdragon 平台 sdm710、8 核、5.54 GiB RAM、应用版本及签名。

两台设备直接解析业务域名失败，本次用仅允许指定 API 的 CONNECT 测试隧道，保持正常 HTTPS 主机名、证书链和 SPKI 验证。没有完成手机直连蜂窝网络/GPS/基站关联验收。硬件证明返回 Keystore -10003，当前体验应用为签名模式，不声称 TEE/StrongBox 验证成功。Go 集成测试覆盖未登录首次密钥注册、重复启动 ID 不变、启动报告不能授权其他账号业务、错误应用及未认证管理访问被拒绝。

更完整的数据源与限制见 environment-0.5.md。此版本适合接入试点；硬件矩阵、真实定位与基站样本、容量及误杀率仍需验收。
