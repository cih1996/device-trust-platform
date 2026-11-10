# 0.4 安全协议

## 通讯与身份

设备上报使用 application/octet-stream 的 DTP4 二进制加密帧。密文内部为严格字段 JSON，控制接口与响应为 HTTPS JSON。二进制编码本身不提供保密性，安全性来自标准密码算法、TLS 与服务端业务约束。

TLS 保留平台证书链和主机名校验，在握手期间验证 SHA-256 SPKI 固定值，支持多个值用于轮换。Android 默认不信任用户安装 CA；体验 APK 只对 localhost 加载交付包的专用公开 CA。未设置证书校验绕过或重定向。

Keystore 生成 P-256 / SHA256withECDSA 签名密钥。JNI 编排 ECDH、HKDF、AES-GCM、组帧和签名，调用平台密码提供者，私钥保持为 Keystore 对象。服务端不向客户端发放通用共享签名密钥。是否有硬件支持以服务端证明校验结果为准。

## 帧格式

长度字段为网络字节序，最大帧 256 KiB。

| 偏移 | 长度 | 内容 |
| --- | --- | --- |
| 0 | 4 | ASCII DTP4 |
| 4 | 1 | 1 注册密钥；2 设备报告 |
| 5 | 24 | 挑战 ID 原始字节 |
| 29 | 32 | 签名公钥 SPKI 的 SHA-256 |
| 61 | 65 | 临时 ECDH P-256 未压缩公钥 |
| 126 | 12 | GCM nonce |
| 138 | 4 | 密文与 16 字节 GCM tag 的总长度 |
| 142 | 可变 | AES-256-GCM 密文与 tag |
| 后续 | 2 | ECDSA DER 签名长度 |
| 后续 | 可变 | SHA256withECDSA 签名 |

ECDH P-256 共享值经 HKDF-SHA256，salt 为挑战的 32 字节 nonce，info 为 device-trust/frame/v1，输出 32 字节。AAD 是完整 142 字节头。签名覆盖头、密文及 tag。每次组帧生成新的临时 ECDH 密钥和 GCM nonce。

## 防重放与绑定

业务服务器认证账号并构造订单、活动等上下文，使用私有应用密钥调用 POST /v1/challenges。挑战绑定应用、账号、业务 eventId、类型、IP、上下文和已注册 keyId，有效期 3 分钟。挑战响应不含服务端私钥。

初次或注册过期时，SDK 根据挑战生成 Keystore 密钥，类型 1 提交证书链与持钥签名；类型 2 提交报告。一个挑战只接受一种注册和一份报告内容，相同重试 200，不同内容 409，过期 410。服务端验证签名、密钥、解密完整性和业务绑定，以 keyId 代替客户端可改的安装 ID。

业务通过 /v1/events 评估，替换账号或订单会被拒绝。新应用拒绝旧版无签名 /v1/reports；旧应用保留显式兼容配置。相同密文重试不会产生第二次事件或执行。

## 设备真实性

后台“接入保护”可以开启硬件密钥验证，配置正式 APK 签名摘要、最低系统补丁和注册有效期。启用后证明不满足就拒绝，不降为签名模式。

校验独立可信根、X.509 链、Google 撤销状态、nonce、TEE/StrongBox 级别、生成来源、签名用途、SHA-256、锁定的 Verified Boot、包名和 APK 签名摘要。选取靠近根的证明扩展，拒绝带委托叶证书的链；当前不支持该类证明。撤销快照超过 24 小时拒绝请求，至少每日刷新；根更新后重启平台。

proof.level 的 signature_only 仅表示持钥签名；tee/strongbox 表示硬件密钥证明通过。enrolledAt 是注册时间，verifiedAt 是本次报告校验时间。证明的启动状态描述注册时状态，不证明后续所有运行行为或原始采集字段真实。卸载、清除数据、重新注册可能更换 keyId，未实现永久物理设备身份。

## 处置接口

policy.mode 支持 shadow 观察与 enforce 执行；执行模式要求签名上报。

- POST /v1/decisions/claim：应用密钥认证，传 appId、业务 eventId、operationId。verify/hold 在批准前拒绝。一个事件只准一个操作；同操作重试返回原授权，不同操作 409。首次领取在事件或批准后 5 分钟内，附 expiresAt。
- POST /v1/decisions/resolve：业务完成真实核验后提交 approved/rejected 和唯一 resolutionId；管理员也可在事件详情审批。最终结论不可覆盖，冲突 409。
- 业务数据库须按 eventId/operationId 原子幂等提交，仅在有效授权内执行实际业务。
- POST /v1/executions：传 claim token、operationId、receiptId、succeeded/failed 和 reference；相同回执可以恢复重试，冲突 409。
- POST /api/device-keys/revoke：停用后拒绝后续报告与首次授权领取；已发出授权由业务侧管理截止与提交。

claim 重试可返回已过期原凭据以补齐已执行业务的回执，业务不能据此启动新效果。演示服务已区分“已提交，补回执”和“未执行，授权过期”。真实支付仍须有自身幂等、回调验签和对账。

## 混淆与边界

xollvm v0.12.0 / LLVM22 实际编译四种 ABI。核心函数标注字符串加密、控制流、虚假分支、指令替换和 shield；各 pass 实际变换以交付 metrics 为准，不承诺每个函数应用全部 pass。只导出 JNI_OnLoad，动态注册 JNI，R8 混淆与收缩 Java，SO 手动去符号并启用 RELRO/NOW/noexecstack。

被控制设备仍可能在加密前读数据或调用合法密钥。混淆提高分析成本，不能承诺永不可抓包、逆向或绝对防伪。设备证明、业务认证、限额与处置需要共同使用。

依据：[Android 密钥证明](https://developer.android.com/privacy-and-security/security-key-attestation)、[AOSP 格式](https://source.android.com/docs/security/features/keystore/attestation)、[Google 根](https://android.googleapis.com/attestation/root)、[撤销状态](https://android.googleapis.com/attestation/status)、[xollvm](https://github.com/und3ath/xollvm/releases/tag/v0.12.0)。
