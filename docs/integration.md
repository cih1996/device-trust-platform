# 接入协议 0.1

## Android

将 `sdk-release.aar` 放入宿主 libs 目录并通过 `implementation files('libs/sdk-release.aar')` 引入。最低 API 23。库仅声明 INTERNET / ACCESS_NETWORK_STATE。无 ContentProvider 自动初始化，不自动弹权限，不持续后台采集。

Java（由宿主自己的后台执行器调用；错误交给宿主直接展示 / 记录）：

```java
DeviceTrust.Options options = new DeviceTrust.Options("应用ID", false);
// 仅在用户同意相关采集、且需要该风控场景时执行。
JSONObject report = DeviceTrust.collect(context, options, true);
String reportId = DeviceTrust.upload("https://测试服务域名/v1/reports", report);
// 将 reportId 随业务请求传给自己的业务服务端。
```

Kotlin（API 相同，不要求引入协程依赖）：

```kotlin
val options = DeviceTrust.Options("应用ID", false)
val report = DeviceTrust.collect(context, options, consentGranted)
val reportId = DeviceTrust.upload("https://测试服务域名/v1/reports", report)
```

`includeAndroidId` 为显式字段开关。collect 是同步方法，会写入本地随机 ID，应同样放后台调用，并从一个集成入口调用。upload 拒绝主线程与 HTTP；返回原始 HTTP 状态与响应内容，不做自动重试和错误替换。开发用 HTTPS 入口由测试环境提供，不在 SDK 中关闭证书检查。宿主如撤回同意应停止调用，按自身数据治理流程删除已采集记录。后台删除功能尚未实现。

## 设备报告

`POST /v1/reports` 是公开采集接口，只接收风险线索，不提供认证。appId 为公开标识。信号状态为 available / permission_denied / unavailable / disabled；只有 available 附值。SDK 0.1 的基础字段为 manufacturer、model、sdkInt、securityPatch、buildFingerprint、debugger、emulatorHint、transport、vpn、operator；androidId 可选。敏感和未实现模块明确 disabled。

```json
{
  "appId": "应用ID",
  "installationId": "客户端本地随机ID",
  "package": "com.example.app",
  "signals": {
    "debugger": {"status":"available","value":false},
    "vpn": {"status":"unavailable","reason":"no active network"}
  }
}
```

返回 `201 {"reportId":"..."}`。报告以服务端接收时间为准，事件引用有效期 15 分钟。package 匹配只是一致性判断，客户端可以伪造。观察到的对端 IP 不信任 X-Forwarded-For；部署测试反向代理时会显示代理 IP，后续需明确可信代理配置。

## 业务事件

自己的业务后端完成用户认证后，调用 `POST /v1/events`，头部 `Authorization: Bearer 应用密钥`。不要将密钥放入 Android App。

```json
{
  "appId":"应用ID",
  "reportId":"SDK返回的报告ID",
  "accountId":"业务侧账号代号",
  "type":"login",
  "clientIp":"203.0.113.10"
}
```

clientIp 可选，应由业务后端按自身可信代理规则确认，不能直接转发用户自报 IP。accountId 建议用业务内部代号，不传手机号等无关数据。

返回 201，包含 id、appId、reportId、accountId、installationId、type、clientIp、receivedAt、score、decision、reasons、modelVersion。decision 为 observe 或 review。当前不自动阻断。

规则 `rules-0.1`：调试 +15、简单模拟器线索 +20、VPN +5、包名不匹配 +30；同应用同 installationId 在 24h 内至少四个不同账号 +35，一分钟内已有至少十个事件 +35；总分封顶 100，达到 30 建议关注。规则分数尚未经实测校准。事件窗口使用服务器时间，跨应用不合并；并发请求按各自历史快照评估。业务重试会重复写事件，正式试点前需要加入业务事件 ID 幂等。

常见状态：400 请求或报告过期；401 凭据无效；403 跨应用报告；404 记录不存在；500 原始存储 / 解码错误。单次错误直接返回，批量调用方应逐项记录结果，继续其他请求。

## 管理接口

所有 `/api/` 接口使用 `Authorization: Bearer 管理口令`。当前一个管理员工作空间。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | /api/apps | 应用列表，不含密钥摘要 |
| POST | /api/apps | 创建；body: name、package；返回 app 与一次性展示的 key |
| POST | /api/apps/{id}/rotate-key | 生成新密钥，旧密钥立即失效 |
| GET | /api/overview?appId=... | 最近 24h 统计、最新 100 条事件和最新出现的 100 个设备画像；省略 appId 为全部应用 |
| GET | /health | 服务就绪 |

管理口令来自 `DTP_ADMIN_TOKEN`，至少 24 字符。应用密钥由密码学随机数生成，存 SHA-256 摘要。当前无多用户登录、角色、操作审计与自助找回，属于本地开发管理入口。

overview 的 profiles 包含 appId、installationId、firstSeen、lastSeen、reportCount、eventCount、accountCount 和 latestReport（含信号状态）。画像计数覆盖当前全部留存历史；顶部 stats 覆盖最近 24 小时。设备画像以应用内客户端 ID 关联，不意味着已验证物理设备身份。
