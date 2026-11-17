# 业务服务端接入 · 0.7.5

本文写给接入 DeviceTrust 的业务后端：App 怎样把设备交给风控，后端怎样查询设备环境、评估业务事件，以及同一开发者的多个 App 怎样关联到同一台手机。

所有判断都在服务端完成。App 只负责采集和签名上报，结论不经过 App。

## 1. 基本概念

| 名称 | 谁持有 | 说明 |
|---|---|---|
| appId | App、业务后端 | 在工作台“应用管理”创建应用得到，公开 |
| appKey | 仅业务后端 | 创建时只显示一次；所有 `/v1/*` 后端接口以 `Authorization: Bearer <appKey>` 调用 |
| deviceId | 平台签发 | 一个 App 安装的设备密钥（Android Keystore）登记后得到；同一 App 内稳定，不能作为登录或授权凭据 |
| linkId | 平台计算 | 同一台手机在你多个 App 中的设备归为一组，见第 6 节 |
| eventId | 业务后端 | 一次业务请求的唯一 ID，由后端生成并持久保存，用于幂等 |
| reportId | 平台签发 | 一次签名上报；App 上传后得到，交回业务后端查询 |

## 2. 整体流程

```
App 启动（用户同意后）
  SDK.initialize ──► 业务后端 POST /device/prepare ──► 平台 POST /v1/challenges
                 ◄── challenge ◄──────────────────────
  SDK 签名、加密上报 ──► 平台 /v1/secure ──► 返回 deviceId、reportId

用户执行登录、注册、领券、支付等操作
  App ──► 业务后端：认证账号，生成 eventId、requestHash
          业务后端 ──► 平台 POST /v1/challenges（绑定账号、eventId、requestHash）
  App 采集并上报 ──► 平台 /v1/secure ──► reportId
  App ──► 业务后端：提交 reportId
          业务后端 ──► 平台 POST /v1/verdicts  查询设备环境
                   └─► 平台 POST /v1/events    评估业务事件（可选）
          业务后端按结论放行、核验或拒绝
```

挑战只有 3 分钟有效且只能使用一次。挑战绑定了账号、eventId 和 requestHash，偷来的报告无法挪用到别的账号或请求上。

## 3. 准备工作

1. 在工作台“应用管理”创建应用，包名填写 Android `applicationId`，保存 appId 和 appKey。
2. 在“接入保护”填写 APK 签名证书的 SHA-256。可以在 App 中读取，也可以用 `apksigner verify --print-certs app.apk` 查看。填写后平台会比对每次上报的签名，并用硬件证明核对包名和签名（第 5 节）。
3. 你的业务后端实现 `POST /device/prepare`（第 4.1 节）。
4. App 在 Activity 的 `dispatchTouchEvent` 中调用 `TouchAudit.record(event)`，平台据此识别模拟点击（脚本、按键精灵、无障碍手势、`input tap`）：

   ```java
   @Override public boolean dispatchTouchEvent(MotionEvent event) {
       TouchAudit.record(event);
       return super.dispatchTouchEvent(event);
   }
   ```
5. 如需基站、定位与网络的交叉校验，App 取得定位权限后调用 `EnvironmentCollector.collect(..., CELL, LOCATION)`。没有授权时这些规则不参与评分，不会误判。

SDK 采集内容（隐私政策需写明“用于安全风控”）：设备与系统参数、ANDROID_ID、指定风险应用是否安装（固定名单，不读取应用列表）、无障碍服务状态、本机监听端口号、网络类型与运营商；授权后包括基站与定位。

## 4. 接口

所有请求和响应都是 JSON，都需要 `Authorization: Bearer <appKey>`。API 地址为 `https://devicetrust-sdk-api.lingzidev.com`。

### 4.1 启动登记：`/device/prepare`（由你的后端实现）

App 启动时 SDK 调用 `{businessUrl}/device/prepare`，请求体为 `{"appId","installationId","keyId"}`。后端代为申请一个启动挑战，然后把挑战原样返回：

```
POST /v1/challenges
{
  "appId": "APP_ID",
  "eventId": "startup-<随机值>",
  "accountId": "guest:<sha256(installationId)>",
  "type": "device_init",          // keyId 非空时用 device_refresh
  "clientIp": "<App 的公网 IP>",
  "context": {"requestHash": "<sha256 业务意图>"},
  "keyId": "<App 传来的 keyId>"
}
```

返回给 App 的内容：`{"appId": "APP_ID", "platformUrl": "https://devicetrust-sdk-api.lingzidev.com", "challenge": <挑战响应>}`。

这个接口是公开的，只能用来登记设备，不发放任何业务授权，需要限制访问频率。Go 参考实现见 `internal/platform/startup.go`。

### 4.2 业务挑战：`POST /v1/challenges`

用户发起敏感操作时，后端先认证账号，再申请挑战：

```json
{
  "appId": "APP_ID",
  "eventId": "login-20261116-0001",
  "accountId": "u_10086",
  "type": "login",
  "clientIp": "203.0.113.10",
  "context": {"requestHash": "REQUEST_SHA256"},
  "keyId": "App 上传的 keyId，首次为空"
}
```

- `requestHash` 是后端对“方法 + 路径 + 账号 + 完整业务参数”计算的 SHA-256，不能接受 App 传来的值。算法见 `pkg/businessguard.HashRequest`。
- 把响应**原样**交给 App，App 调用 `SecureReporter.upload` 后会得到 `reportId`。响应里可能带 `lanTargets`（同局域网其他设备的内网地址，供 SDK 探测），不要删减字段。

### 4.3 查询设备环境：`POST /v1/verdicts`

拿到 App 交回的 reportId 后查询，这是大多数业务需要的接口：

```json
{"appId": "APP_ID", "reportId": "REPORT_ID", "eventId": "login-20261116-0001", "accountId": "u_10086"}
```

`eventId` 和 `accountId` 必须与申请挑战时一致，否则返回 403；报告超过 15 分钟返回 410。响应示例：

```json
{
  "reportId": "…",
  "deviceId": "…",
  "linkId": "…",
  "keyStatus": "active",
  "riskScore": 85,
  "riskLevel": "high",
  "envScore": 85,
  "businessScore": 0,
  "factors": [
    {"code": "boot_state_spoofed", "points": 40, "source": "client"},
    {"code": "network_interface_mismatch", "points": 30, "source": "client"}
  ],
  "app": {"package": "com.example.app", "signer": "matched", "attestation": "none", "issue": "boot_unlocked"},
  "linked": {"devices": 1, "apps": 1, "maxRiskScore": 90, "highRisk": 1},
  "lan": {"devices": 6, "adbPeers": 2},
  "observedIp": "113.87.130.104",
  "region": {"country": "中国", "province": "广东省", "city": "深圳市", "isp": "电信"},
  "reportedAt": "2026-11-16T10:00:00Z"
}
```

| 字段 | 含义 |
|---|---|
| riskScore / riskLevel | 设备总分 0–100，等于 max(envScore, businessScore)；low < 30 ≤ medium < 70 ≤ high |
| envScore | 本次上报的设备环境分 |
| businessScore | 该设备 7 天内业务事件的最高分 |
| factors | 命中的规则，代码表见第 8 节 |
| keyStatus | `active`，或 `revoked`（已在工作台停用） |
| app.signer | `matched` 签名一致；`mismatch` 签名不符（疑似重打包）；`unknown` 未上报；`unconfigured` 工作台未填写证书 |
| app.attestation | `tee` / `strongbox`：硬件证明确认包名、签名且启动链锁定；`none`：没有通过 |
| app.issue | 硬件证明未通过的原因：`boot_unlocked`、`app_mismatch`、`revoked`、`unverified`、`keystore_failed`（设备 Keystore 无法生成证明，SDK 改用无证明密钥登记；仅在未开启“要求硬件证明”时出现） |
| linked | 这台手机在你其他 App 中的设备数、涉及应用数、其中最高风险分和高风险设备数 |
| lan | 24 小时内与它处于同一局域网的其他手机数（跨你的所有 App）；`adbPeers` 为 SDK 探测到开放 ADB 无线调试的同网设备数 |

建议处理方式：`high` 时拒绝或要求更强的核验（短信、人脸）；`medium` 时加验证；`app.signer = mismatch` 或 `app.issue = app_mismatch` 时直接拒绝；`linked.highRisk > 0` 表示这台手机在别的 App 中出现过高风险，可以从严处理。

### 4.4 按设备 ID 查询历史：`POST /v1/devices/query`

```json
{"appId": "APP_ID", "deviceId": "DEVICE_ID"}
```

返回这台设备最近一次上报的结论，格式与 4.3 相同。它只是历史档案，不能证明设备此刻在场；实时请求请用 4.3。

### 4.5 业务事件评估：`POST /v1/events`

需要账号、活动、支付等业务维度的风险时（多账号、同设备集中注册、频繁领取、支付工具轮换等），提交事件：

```json
{
  "appId": "APP_ID", "eventId": "login-20261116-0001", "reportId": "REPORT_ID",
  "accountId": "u_10086", "type": "register", "clientIp": "203.0.113.10",
  "context": {"requestHash": "REQUEST_SHA256", "campaignId": "…", "amountMinor": 1000, "currency": "CNY"}
}
```

账号、类型、IP 和 context 必须与挑战一致。响应包含 `score`、`action`（observe / verify / hold）、`reasons` 和 `mode`。执行模式下还需要调用 `/v1/decisions/claim` 领取执行授权，再用 `/v1/executions` 回执，详见 `docs/secure-protocol.md` 与 `docs/business-risk.md`。

## 5. 应用身份校验

同一个 appId 只应由你自己签名的 App 使用。平台从两个层面判断：

1. **签名比对**（每次上报）：SDK 上报 APK 签名证书摘要，平台与“接入保护”中填写的证书比对。不一致时命中 `app_signature_mismatch`（40 分），verdict 中 `app.signer = mismatch`。这一层由 App 自报，被 Hook 时可能伪造。
2. **硬件证明**（设备登记时）：SDK 生成密钥时总会附带平台的挑战值，Android Keystore 会签发证书链。平台用 Google 根证书和每日更新的吊销列表验证证书链，并读取硬件写入的包名、签名证书和启动状态：
   - 包名或签名不符：`attested_app_mismatch`（60 分）
   - 证书已吊销（例如泄露的 keybox）：`attestation_revoked`（40 分）
   - Bootloader 未锁定：`attested_boot_unlocked`（25 分）
   - 全部通过：verdict 中 `app.attestation = tee` 或 `strongbox`

默认情况下证明未通过只扣分，不拒绝上报。在“接入保护”开启“硬件证明”后，未通过证明的设备会被直接拒绝，适合支付等高风险业务，开启前应先在目标机型上验收。

## 6. 跨应用设备关联

每个 App 在手机里有各自的 Keystore 密钥，所以 deviceId 按 App 区分。平台另外计算 linkId，把同一台手机在你多个 App 中的设备归为一组：

| 依据 | 条件 | 说明 |
|---|---|---|
| 同一开发者设备标识 | 各 App 使用**同一个签名证书** | Android 8 起 ANDROID_ID 按签名证书和用户隔离；同一签名的 App 在同一台手机上读到相同的值，换签名的 App 读不到 |
| 同一次开机 | 硬件参数一致，且开机时刻相差不超过 2 秒 | 不依赖签名；两个 App 需在同一次开机期间上报过 |

关联一经建立就会保存，手机重启或 App 重装后依然有效（重装后得到新的 deviceId，但 linkId 不变）。工作台“设备 → 关联与轨迹”会列出同一台手机在其他 App 中的设备。

注意事项：

- 关联需要 SDK 0.7 及以上版本。SDK 会采集 ANDROID_ID，请在隐私政策中写明“设备标识用于安全风控”。
- 关联依据是客户端观测，可能被改机工具篡改。它是风险线索，不能作为身份证明。
- verdict 的 `linked` 只给出汇总，不会把其他 App 的 deviceId 透露给当前 App 的后端。

## 6.1 局域网聚集（工作室识别）

SDK 0.7.4 起上报 `lan`：Wi-Fi / 有线网络（包括被系统标成移动网络的 eth0，常见于箱式和云手机）的内网地址、网段、网关、DNS。工作台设备列表和详情会显示内网 IP，可按内网 IP 搜索，方便对应到具体手机。

平台用两种依据判断“同一局域网”：

| 依据 | 说明 |
|---|---|
| 路由器 BSSID | 需要 App 授予定位权限并采集 Wi-Fi 信息；不同出口的设备只要连同一台路由器也能归到一起 |
| 出口 IP + 内网网段 + 网关 | 不需要额外权限；同一运营商 NAT 地址后面常有许多家庭都用 192.168.1.0/24，所以必须三者都相同 |

服务端引导探测：老设备申请挑战时，平台在响应里附上 `lanTargets`，即最近 2 小时同一局域网内其他 SDK 设备的内网地址（最多 32 个）。SDK 只探测其中**位于本机网段内**的地址：ICMP 可达性，以及 ADB 无线调试端口 5555，总耗时不超过 3 秒。不会扫描网段内的其他设备。

| 结果 | 规则 |
|---|---|
| 24 小时内同一局域网 ≥ 5 台手机 | `lan_device_cluster` 20 分 |
| 同网设备开放 ADB 无线调试（群控常用） | `lan_adb_peers` 30 分 |
| 同网设备可达 | 记为“同网设备探测”一致，确认确实在同一网络 |
| 网关可达但同网设备全部不可达 | 记为“疑似 AP 隔离”，不计分（酒店、公司网络也会隔离），但聚集规则照常生效 |

注意：

- 公司、学校、酒店等公共 Wi-Fi 下多台设备聚集是正常的，`lan_device_cluster` 只是线索，宜结合 `linked`、业务规则一起判断。
- 各手机分别用移动数据或独立代理时没有局域网可言，需要依靠出口、基站、开机时刻关联等其他规则。
- 隐私政策需写明：为安全风控，会采集设备内网地址，并检测同一局域网内其他已安装本 SDK 的设备是否可达。
- Android 正在逐步收紧应用访问局域网的权限，后续目标 API 版本可能需要申请局域网访问权限后才能探测；届时探测结果会显示为不可用，不影响其他规则。

## 6.2 基站库

平台使用 OpenCelliD 的中国（MCC 460）众包基站数据，约 21 万个基站，主要是 LTE 和 NR。基站坐标是众包测量的平均值，并不完整，所以：

- 库里查不到不算风险；样本少于 3 个的基站不参与距离比较。
- 只有注册中、两分钟内测得的基站参与比较，且需要 App 采集基站（需定位权限）。
- 数据每月自动更新一次，下载后先完整校验，通过才替换。

数据来源：OpenCelliD（opencellid.org），CC BY-SA 4.0。

## 7. 其他语言示例

Go 可以直接使用 `pkg/businessguard`：

```go
guard := businessguard.Client{BaseURL: "https://devicetrust-sdk-api.lingzidev.com", AppID: appID, AppKey: appKey}
verdict, err := guard.Verdict(ctx, reportID, eventID, accountID)
if err != nil { return err }               // 原样记录错误，不当作“通过”
if verdict.RiskLevel == "high" || verdict.App.Signer == "mismatch" { /* 拒绝或加验证 */ }
```

Java（JDK 11+）：

```java
HttpRequest request = HttpRequest.newBuilder(URI.create(api + "/v1/verdicts"))
    .header("Authorization", "Bearer " + appKey)
    .header("Content-Type", "application/json")
    .POST(HttpRequest.BodyPublishers.ofString(
        "{\"appId\":\"" + appId + "\",\"reportId\":\"" + reportId +
        "\",\"eventId\":\"" + eventId + "\",\"accountId\":\"" + accountId + "\"}"))
    .build();
HttpResponse<String> response = HttpClient.newHttpClient().send(request, HttpResponse.BodyHandlers.ofString());
if (response.statusCode() != 200) throw new IllegalStateException(response.body());
```

Node.js（18+）：

```js
const response = await fetch(`${api}/v1/verdicts`, {
  method: 'POST',
  headers: { Authorization: `Bearer ${appKey}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ appId, reportId, eventId, accountId }),
})
if (!response.ok) throw new Error(`${response.status} ${await response.text()}`)
const verdict = await response.json()
```

请求失败时不要把设备当作“安全”处理，应按业务的失败策略拒绝或要求重试。

## 8. 规则代码

分值为试行值，会随样本校准。

| 代码 | 含义 | 分值 |
|---|---|---|
| boot_state_spoofed | 启动状态被篡改（多处读数矛盾） | 40 |
| bootloader_unlocked | Bootloader 已解锁 | 20 |
| root_properties / root_mounts | Root 框架属性 / Root 挂载痕迹 | 35 / 40 |
| selinux_domain | 应用不在正常的 SELinux 域 | 30 |
| build_tags_mismatch | 系统签名标记矛盾 | 25 |
| network_interface_mismatch | 声称的网络类型与实际网卡不符 | 30 |
| sim_network_mismatch | SIM 与注册网络矛盾 | 20–25 |
| cellular_ip_carrier_mismatch | 移动数据出口运营商与 SIM 不符 | 20 |
| hook_framework / hook_threads | Hook 框架痕迹 / 注入线程 | 40 / 30 |
| native_debugger / debugger | 调试器附加 | 25 / 15 |
| emulator_traits / emulator_hint / x86_runtime | 模拟器线索 | 35 / 20 / 15 |
| virtual_container / secondary_user | 多开容器 / 分身空间 | 30 / 5 |
| app_debuggable | 应用为可调试版本 | 30 |
| app_signature_mismatch | 上报签名与登记证书不符 | 40 |
| attested_app_mismatch | 硬件证明：包名或签名不符 | 60 |
| attestation_revoked | 硬件证明证书已吊销 | 40 |
| attested_boot_unlocked | 硬件证明：Bootloader 未锁定 | 25 |
| http_proxy / vpn / adb_enabled | 代理 / VPN / USB 调试 | 10 / 5 / 5 |
| risk_app_hook / risk_app_root | 安装 Hook 框架管理器（LSPosed、Xposed 等）/ Root 管理器（Magisk、KernelSU、APatch） | 35 / 30 |
| risk_app_virtual / risk_app_automation / risk_app_cheat | 多开与虚拟机 / 自动化脚本（Auto.js、按键精灵、触动精灵等）/ 改内存工具 | 25 |
| risk_app_mock_location / risk_app_capture | 虚拟定位 / 抓包工具 | 20 |
| risk_app_privilege / risk_app_remote_control | Shizuku / 远程控制应用 | 10 |
| java_hook_framework | 进程内可见 Xposed / LSPosed / Pine / SandHook 类或调用栈 | 40 |
| injected_code | 从 /data/local/tmp、memfd 或已删除文件加载的可执行代码 | 35 |
| frida_server | 本机端口响应 Frida 握手（任意端口） | 50 |
| frida_default_port / debug_server_port | Frida 默认端口 27042 / IDA 调试端口 23946 开放 | 30 |
| exposed_listening_ports | 本机有端口对局域网开放（远程控制、调试桥常见） | 15 |
| accessibility_automation | 第三方无障碍服务可模拟手势 | 20 |
| hidden_accessibility_client | 无障碍已开启但没有任何登记服务（UiAutomator、Appium 等 UI 自动化） | 25 |
| injected_touches | 触摸事件为注入的模拟点击（需接入 TouchAudit） | 30 |
| monkey_test | 系统处于 Monkey 测试模式 | 30 |
| vendor_packages_missing / foreign_vendor_packages | 品牌与厂商系统应用不符（改机）；仅对国内 SIM 或 IP 生效 | 25 |
| datacenter_egress | 出口 IP 属于云服务器或机房（代理、云手机） | 30 |
| ip_outside_sim_country | 大陆 SIM 未漫游，出口在境外或港澳台 | 20 |
| cell_operator_mismatch / cell_sim_country_mismatch | 注册基站与系统报告的运营商 / SIM 国家不符（运营商被篡改） | 25 / 20 |
| lan_device_cluster | 24 小时内同一局域网 ≥ 5 台手机 | 20 |
| lan_adb_peers | 同局域网设备开放 ADB 无线调试端口 5555 | 30 |
| location_jump | 同一手机两次定位间的速度超过 900 km/h（且距离超过 100 km） | 25 |
| mock_location / cell_gps_mismatch / ip_gps_region_mismatch | 系统标记模拟定位 / 定位与注册基站相距超过 max(3 倍覆盖半径, 20 km) / 网络与定位区域不一致 | 20 / 25 / 10 |
| cell_identity_implausible | 国内 LTE/NR 注册基站不在基站库中，且编号像模拟器占位值（Cell ID < 65536，或 1234、5678、9999 这类数字） | 25 |
| cell_ip_region_mismatch | Wi-Fi 下注册基站所在省份与宽带出口省份不同（移动数据不比较：省际漫游从归属省出口） | 15 |
| cell_device_cluster | 1 小时内 ≥ 10 台手机注册在同一基站（插卡工作室） | 15 |
| 其他资源验真类 | 机型参数、内存、系统应用签名等矛盾 | — |

资源验真类（运营商、出口、基站、定位、厂商系统应用、局域网）合计最多计 50 分，避免单一数据源决定结论。
| 业务类 | multiple_accounts_24h、registration_device_cluster、campaign_*、payment_* 等 | 见 business-risk.md |

## 9. 错误码

| 状态 | 处理 |
|---|---|
| 400 | 请求格式或必填字段错误，查看 `error` |
| 401 | appKey 错误 |
| 403 | 报告属于其他应用或其他业务请求、设备密钥已停用、要求硬件证明但未通过 |
| 404 | 应用、报告或设备不存在 |
| 409 | 同一 eventId 被用于不同内容 |
| 410 | 挑战或报告已过期，让 App 重新采集 |

服务端的原始错误会原样返回。排查时请记录 eventId、reportId、时间和完整错误，不要附带 appKey。
