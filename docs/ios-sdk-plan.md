# iOS SDK 方案

目标：给 iOS App 提供与 Android 同等用法的设备风控 SDK，服务端沿用同一套挑战、上报、事件评估和 enforce 授权流程，工作台统一展示。

## 1. 总体原则

- 协议不分叉：iOS 沿用 DTP4 加密帧（ECDH P-256 + HKDF + AES-256-GCM + ECDSA 签名），服务端解帧、防重放和业务绑定代码不变。
- 判定只在服务端：SDK 只采集和签名，不做风险决定。
- 只用苹果公开 API，不读取 IDFA、通讯录、照片等隐私数据，不需要额外权限弹窗，可以过 App Store 审核。

## 2. 与 Android 的对应

| Android | iOS | 说明 |
|---|---|---|
| Keystore P-256 签名密钥 | Secure Enclave P-256 密钥（不可导出） | 签名格式同为 DER ECDSA，keyId 同为 SPKI 的 SHA-256 |
| Keystore 硬件证明（Google 根） | App Attest（苹果根，DCAppAttestService） | 证明"真 iPhone + 正版 App"，服务端离线校验，无吊销列表依赖 |
| APK 签名摘要校验 | App Attest 中的 Team ID + Bundle ID（rpIdHash） | 重签名、二次打包无法通过 |
| ANDROID_ID（重装不变） | Keychain 设备密钥（ThisDeviceOnly，卸载重装后仍在） | 用于"清数据后同一设备 ID"的匹配 |
| Widevine ID | identifierForVendor + 可选 DeviceCheck 两位标记 | DeviceCheck 抹机也不丢，需开发者账号的 DeviceCheck 私钥（第三阶段） |
| 硬件画像（型号、存储、屏幕） | 同字段：机型标识（如 iPhone17,1）、存储容量、屏幕像素 | 服务端 hardwareProfile 直接复用 |
| Root / Magisk | 越狱检测（rootless、rootful、palera1n、Dopamine） | |
| Frida / Xposed Hook | 动态库注入、Frida、Substrate/ElleKit、函数被替换 | |
| 调试检测 | sysctl P_TRACED + 可选 PT_DENY_ATTACH | |
| 模拟器 | 模拟器（编译期和运行期） | 模拟器无 Secure Enclave、无 App Attest |
| 代理、VPN | 系统代理设置、utun/ipsec/ppp 网卡 | |
| Java 与 NDK 交叉比对 | Swift(Foundation) 与 C(libc/syscall) 交叉比对 | Hook 只改一层会露出矛盾 |
| 无障碍、自动点击、已装应用、基站、局域网扫描 | 无对应 | 苹果限制，相关规则在 iOS 上跳过，不扣分 |

## 3. 注册与上报流程

1. 业务后端照旧调用 `POST /v1/challenges` 领取挑战（不变）。
2. 首次注册（帧类型 1）：
   - SDK 在 Secure Enclave 生成 P-256 签名密钥。
   - SDK 生成 App Attest 密钥，并调用 `attestKey`，clientDataHash = SHA256("device-trust/appattest/v1" ‖ 挑战 nonce ‖ 签名公钥 SPKI)。这样苹果证明的就是"这把签名密钥属于这台真机上的正版 App，且是为这次挑战生成的"。
   - 加密载荷：`{"platform":"ios","publicKey":SPKI,"appAttest":{"keyId":…,"attestation":…}}`；App Attest 不可用（模拟器、旧机型、系统故障）时改为 `attestationError`，与 Android 相同处理：开启"要求硬件证明"的应用直接拒绝，否则记为 signature_only。
3. 服务端校验 App Attest：
   - x5c 证书链到苹果 App Attestation 根证书（根证书固定在仓库里，不在线获取）；
   - nonce 扩展（OID 1.2.840.113635.100.8.2）= SHA256(authData ‖ clientDataHash)；
   - rpIdHash = SHA256(TeamID.BundleID)，需与后台配置一致；
   - counter = 0，credentialId 与凭证公钥一致；
   - aaguid 区分开发环境和生产环境，后台可配置是否接受开发环境。
   - 通过后 proof.level = `app_attest`，并保存 App Attest 公钥和计数器。
4. 设备报告（帧类型 2）：Secure Enclave 签名帧（与 Android 相同）；载荷内附带一次 App Attest 断言（assertion），clientDataHash 绑定挑战 nonce。服务端验证断言签名并要求计数器递增，用来发现密钥被复制到别的设备。
5. 事件评估、claim、resolve、executions 全部不变。

服务端需要一个最小 CBOR 解码器（约百行，标准库实现，不引入依赖）。

## 4. iOS 采集信号

字段名与 Android 相同的直接复用（model、manufacturer、osInfo、storage、display、memory、battery、httpProxy、vpn、activeInterface、networkSnapshot、debugger、emulatorHint、application），新增：

| 信号 | 内容 |
|---|---|
| platform | `ios` |
| keychainId | Keychain 中的随机设备密钥摘要（按 App 隔离） |
| vendorId | identifierForVendor |
| bootTime | 开机时间（kern.boottime），用于重装匹配 |
| jailbreak | 越狱文件、目录、越狱商店、沙盒越界写入、fork 可用性、可疑符号链接；Swift 与 C 各读一遍 |
| injection | 已加载动态库中的 Frida、Substrate、ElleKit、libhooker、TweakInject、SSL Kill Switch、Shadow 等；DYLD_INSERT_LIBRARIES；本地 27042 端口 |
| codeIntegrity | Bundle ID、Team ID、可执行文件是否被解密（App Store 包 cryptid=0 说明被砸壳）、嵌入描述文件类型 |
| hookIntegrity | 关键 libc / ObjC 函数地址是否落在原始系统库内 |

服务端新增 iOS 规则码（初始分值可在后台调）：

| 规则 | 级别 | 初始分值 |
|---|---|---|
| jailbreak_detected 越狱 | 硬 | 60 |
| injected_library 动态库注入或 Frida | 硬 | 60 |
| hook_detected 系统函数被替换 | 硬 | 50 |
| debugger_attached 调试中 | 硬 | 40 |
| simulator 模拟器 | 硬 | 60 |
| binary_decrypted 包被砸壳 | 硬 | 50 |
| app_attest_failed 证明失败 | 硬 | 30 |
| app_attest_counter 断言计数回退 | 硬 | 60 |
| layer_mismatch Swift 与 C 结果矛盾 | 硬 | 40 |

网络类规则（代理、VPN、IP 地区等）、业务规则、良好使用恢复、设备画像全部复用。

## 5. SDK 接口（Swift，iOS 15+）

```swift
let config = DeviceTrust.Config(appId: "...", businessURL: URL(string: "https://...")!, pins: ["sha256/..."])
let result = try await DeviceTrust.initialize(config: config, consent: true)   // 冷启动登记
try await DeviceTrust.refresh(config: config)                                  // 回前台补报
try await DeviceTrust.report(config: config, challenge: challenge)             // 业务事件
DeviceTrust.deviceId(appId:)
```

- 交付：Swift Package（`binaryTarget` 指向 XCFramework），同时提供 XCFramework zip。
- 证书固定：URLSession 委托中校验 SPKI SHA-256。
- 不在主线程做网络，不弹任何权限框。

## 6. 防逆向

- 检测核心与组帧用 C 编写，用 xollvm 编译（字符串加密、控制流平坦化、虚假分支、间接调用），以静态 XCFramework 交付；Swift 层只做接口转发。
- 去除符号，只暴露必要的 C 入口。
- iOS App 本身经苹果加密，破解者需先砸壳；砸壳后的包会被 codeIntegrity 识别。
- App Attest 校验包名和团队 ID，重签名的包无法通过证明。

## 7. 仓库与构建

- 分支：`ios-sdk`，目录：`ios/`（Swift Package、C 核心、示例 App、构建脚本）。
- Mac：`~/work/devicetrust-ios` 克隆本仓库，用 xcodegen 生成示例工程，`xcodebuild` 构建 XCFramework。
- 示例 App 包名：`com.devicetrust.sample.dev`，开发团队 335JAYJJ85，真机 iPhone 16 Pro 验证。

## 8. 分阶段

| 阶段 | 内容 | 验收 |
|---|---|---|
| 1 服务端 | iOS 注册分支、App Attest 证明与断言校验、CBOR 解码、后台配置 Team ID / Bundle ID、iOS 规则码 | 单元测试（含真机采到的证明样本） |
| 2 SDK 核心 | Secure Enclave 密钥、DTP4 组帧、App Attest、证书固定、基础信号、示例 App | 真机注册 proof.level=app_attest，模拟器为 signature_only，事件评估走通 |
| 3 检测 | 越狱、注入、Hook、调试、砸壳、Swift/C 交叉比对；可选 DeviceCheck | 真机正常机零误报；规则用构造数据测试 |
| 4 加固与交付 | xollvm 编译 C 核心、XCFramework、Swift Package、工作台 iOS 展示、接入文档 | 交给小橙有喜 iOS 集成 |

## 9. 已知限制

- 没有越狱测试机时，越狱检测只能用构造数据和模拟环境验证，上线前最好补一台越狱机实测。
- App Attest 需要在开发者账号里为该 App ID 开通 App Attest 能力；开发签名下产生的是开发环境证明，正式上架后为生产环境证明。
- DeviceCheck 需要开发者账号生成 DeviceCheck 私钥（.p8）并配置到服务端，作为第三阶段可选项。
- iOS 拿不到基站、已装应用、局域网扫描结果，对应规则不参与评分。
