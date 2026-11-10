# Device Trust Platform · 知衡 0.4

独立的 Android 设备风险 SDK、业务处置服务与管理后台，覆盖注册、营销、支付场景的设备线索和账号关联规则。

## 能力

- 二进制加密上报、Keystore 持钥签名、挑战与账号/业务上下文绑定、幂等去重、设备密钥停用。
- 可选硬件证明验证：可信根与撤销状态、TEE/StrongBox、启动锁定状态、包名和 APK 签名摘要。
- 观察与执行模式、核验/审批、执行授权、幂等业务效果与回执。
- NDK 编排签名与加密，四 ABI 经 xollvm 构建，R8 发布 APK；独立 HTTPS 体验业务服务。

启动与构建见 [交付说明](docs/delivery-0.4.md)，协议见 [安全协议](docs/secure-protocol.md)。原有规则和研究见 [业务风险](docs/business-risk.md)、[研究](docs/research.md)、[开源复用](docs/open-source-review.md)。历史验证记录见 [validation](docs/validation.md)，当前范围以 0.4 交付说明为准。

## 结构

- android/sdk：基础采集；sdk-environment：可选环境采集；sdk-rootbeer：可选 RootBeer 适配。
- android/sdk-secure：Keystore、TLS 固定、NDK 二进制协议；sample：注册/奖励/支付体验 App。
- cmd/server：平台；cmd/demo-business：示例业务账本；setup-demo：私密配置初始化；trust-sync：官方证明信任数据刷新。
- internal/platform：Go 标准库服务、存储、规则和测试；web：Vue/Vite 管理工作台。

## 检查

```powershell
go test ./...
go vet ./...
npm.cmd --prefix web test
npm.cmd --prefix web run check
npm.cmd --prefix web run build
```

后端仅用 Go 标准库；基础与 secure Android 模块无第三方运行库。RootBeer0.1.2 独立可选，授权保留在 third_party。第三方线索不直接等同设备可信。

当前为隔离联调版本。体验默认执行模式、签名开启、硬件证明关闭。硬件证明尚待真机验收，风险分数尚未校准，真实业务应先试点评测。未提供永久物理设备标识、绝对抗逆向承诺或真实支付机构对接。

数据逐条原子发布，读取损坏记录直接报错；同事件内容重试返回原结果，冲突 409。首次创建依赖同文件系统硬链接，已在本机 NTFS 验证。当前文件扫描未做生产容量与入口配额验证，关联规则并发使用历史快照。正式环境需接入业务数据库幂等提交、真实核验与支付回调，并完成设备矩阵和误杀率验收。
