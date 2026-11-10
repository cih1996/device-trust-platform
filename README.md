# Device Trust Platform

独立的 Android 设备风险 SDK 与风控平台。当前阶段先研究数美科技公开的设备指纹与反欺诈产品能力，形成有来源的功能对照与产品范围，再开发 SDK、服务端和管理后台。

## 当前版本 0.3

已实现：Android Java SDK（Kotlin 可直接调用）、基础信号显式采集与 HTTPS 上报；Go 多应用与密钥管理、设备报告与画像、业务事件评估、统计；Vue 管理工作台。

0.2 新增：业务事件 eventId 并发去重、可选网络位置采集模块、可选 RootBeer 0.1.2 适配模块。GitHub 候选与实际复用范围见 [docs/open-source-review.md](docs/open-source-review.md)。

0.3 新增：注册 / 营销 / 支付场景关联规则、命中证据与输入覆盖、应用策略阈值、支付结果回流、人工反馈留痕。协议与规则表见 [docs/business-risk.md](docs/business-risk.md)。当前观察模式返回核验 / 暂缓建议，由业务方试点评测后接入实际处置。

研究矩阵、Android 各信号约束和阶段计划见 [docs/research.md](docs/research.md)；接口和 SDK 示例见 [docs/integration.md](docs/integration.md)。

服务端仅使用 Go 标准库；后台运行时依赖 Vue，构建使用 Vite；Android 基础与网络位置模块无第三方运行时依赖，RootBeer 为独立选装。当前为本地联调版本，模型为可解释规则，尚未校准，不应据此自动封禁用户。

## 本地运行

需要 Go 1.26+、Node 22.12+（或受 Vite 支持的更新版本）。PowerShell：

```powershell
cd web
npm.cmd ci
npm.cmd run build
cd ..
$env:DTP_ADMIN_TOKEN = '替换为至少24字符的随机管理口令'
go run ./cmd/server
```

打开 http://127.0.0.1:8099，输入该口令。先创建应用，再接入 SDK 与业务服务端。管理口令仅在页面内存保存；应用密钥只在创建 / 重置时展示，服务端存摘要，必须保存在业务服务端。

默认只监听本机。`go run ./cmd/server -addr 127.0.0.1:8099 -data data -web web/dist` 可指定路径。SDK 上报使用 HTTPS；实机接入需在测试环境提供有效 TLS 入口，不修改生产网关。数据保存在被 Git 忽略的 data/。

## 构建与检查

```powershell
go test ./...
go vet ./...
cd web
npm.cmd run build
cd ../android
.\gradlew.bat :sdk:assembleRelease :sdk-environment:assembleRelease :sdk-rootbeer:assembleRelease
```

Android 使用 JDK 17、Gradle 8.11.1、Android SDK 35 和 AGP 8.9.2；设置 JAVA_HOME、ANDROID_HOME。AAR 输出为 `android/sdk/build/outputs/aar/sdk-release.aar`。已附 Gradle wrapper，首次构建需要下载 Gradle 与 Google / Maven 构建依赖。仓库不绑定机器路径。

## 结构

- `android/sdk`：基础采集与上报 API。
- `android/sdk-environment`：可选 Wi-Fi / 基站 / 位置缓存采集。
- `android/sdk-rootbeer`：可选 RootBeer 单项采集适配。
- `android/sample`：独立采集测试 App，`:sample:assembleDebug` 构建测试 APK。
- `cmd/server`、`internal/platform`：HTTP 服务、文件存储与试运行规则。
- `web`：应用、风险事件和统计工作台。
- `docs`：公开资料研究与接入协议。

## 使用边界

数据文件逐条原子发布，读取损坏记录会直接返回错误。事件按应用与 eventId 去重，首次创建使用同文件系统硬链接原子发布，要求数据目录所在文件系统支持硬链接（已在本机 NTFS 测试）。相同事件内容重试返回原始结果，内容冲突返回 409。当前全量扫描适合开发联调；不同事件的并发规则使用历史快照，后续改为索引存储。公开设备上报接口尚无挑战和限流，试点前补齐。安装 ID 可被清除、伪造或从系统备份恢复。未做跨重装设备识别、硬件证明验证、完整改机检测与模型准确率评测。

验证记录见 [docs/validation.md](docs/validation.md)。

## 目标

- 可复用的 Android SDK，Java/Kotlin API 与按需使用的 NDK `.so` 信号采集。
- 设备与网络环境采集、信号可用性说明、真实性校验和服务端画像。
- 应用创建、密钥管理、风险事件、统计和策略配置后台。
- 对 Android 权限、应用市场政策和个人信息保护要求逐项核实。

## 首步研究来源

- 数美设备指纹产品：https://help.ishumei.com/docs/tw/sdk/overview/
- 数美设备风险能力：https://www.ishumei.com/m/product/bs-post-sdk.html
- 数美 Android 接入文档：https://help.ishumei.com/docs/tw/sdk/android/developDoc/
- 数美渠道流量反作弊：https://help.ishumei.com/docs/tw/channel/api/

研究结论应区分公开可验证功能、需商务确认的功能，以及 Android 普通应用无法可靠获得的数据。不要把 `.so` 当作绕过系统权限或证明设备真实的手段。
