# Device Trust Platform · 知衡 0.7

后台 0.7 重新设计：应用随时切换，风险总览支持 24 小时 / 7 天 / 30 天统计图表，设备列表按设备风险评分排序与筛选，设备详情提供评分构成与 30 天趋势，风险事件支持分页筛选。见 [后台 0.7 说明](docs/console-0.7.md)。

0.6 新增用户同意后的启动登记，无需业务登录即可取得设备 ID；后台提供完整设备档案和参数、签名、网络、关联轨迹。见 [0.6 接入与验证](docs/startup-0.6.md)。

后台 0.6.1 改为账号密码登录，应用管理使用应用卡片进入；风险概览增加活跃、IP、页面停留、风险分布和访问明细。见 [后台说明](docs/console-0.6.1.md)。

0.5 新增跨 Java/NDK 参数与系统应用签名比对、IPv4/IPv6 归属、带精度/时效约束的网络关联引擎、服务端设备 ID、业务请求摘要绑定和 Go 服务端执行关口。两路 ARM64 MI 8 SE 受测环境已运行发布 APK；硬件证明生成失败与实际网络样本缺口均保留。接入与验证范围见 [0.5 说明](docs/environment-0.5.md)。

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
