# 0.1 首版验证记录

日期：2026-11-10；平台：Windows；独立仓库：`D:\工作\android-games\device-trust-platform`。

## 已通过

- Go 1.26.4：`go test ./...`、`go vet ./...`、服务端可执行文件构建。
- 生命周期测试：管理鉴权、创建应用、设备上报、业务评估、多账号规则、密钥轮换即时失效、重新打开存储读取数据、应用统计隔离。
- 24 个并发设备报告写入，记录完整；缺失信号保持 0 分；损坏记录直接返回 500；非法信号状态与过期报告被明确拒绝。
- 画像测试：应用隔离、最新报告、首次出现时间、事件总量与账号去重。
- Android：JDK 17 / Gradle 8.11.1 / AGP 8.9.2 / compileSdk 35，`:sdk:assembleRelease`、`:sdk:lintRelease` 成功；0 errors / 1 warning。警告是可选 ANDROID_ID 的 HardwareIds 提示，保留原始提示，未屏蔽；该字段仅为可选反欺诈线索。无第三方运行时库。
- 后台：Vue 3.5 / Vite 7，`npm run build` 成功；Vue 模板使用官方编译器单独编译通过。
- 本地 HTTP 联调：临时数据目录下创建应用 → 上报设备 → 业务密钥提交四个账号的登录事件 → 查询统计。临时账号 / 凭据 / 数据不进入发布包或 Git。
- 运行服务验证：统计为 4 事件 / 1 活跃设备 / 4 待关注；画像为 4 账号 / 4 事件 / 1 上报。后台 HTML 与构建后 JS 均返回 HTTP 200。

## 环境处理

- 本机 npm 全局代理指向未运行的 127.0.0.1:7890。仅本次命令使用 `--proxy=null --https-proxy=null --no-audit --no-fund --fetch-retries=0 --fetch-timeout=20000`，未修改全局设置。锁文件记录实际依赖版本。
- 仓库中文路径触发 AGP 路径检查。在本仓库 gradle.properties 使用 AGP 提供的 `android.overridePathCheck=true` 后 Java SDK 构建成功；没有移动 / 修改其他项目。工具会打印该选项为 experimental 的提示。

## 尚未验证

- 当前电脑控制环境浏览器列表为空，内置浏览器入口不可用：没有完成实际浏览器交互、截图或移动端视觉验证。构建 / 模板检查不能替代这些验证。
- 没有在用户设备安装 App，没有做 Android 真机采集、权限 / ROM / API 版本矩阵或 HTTPS 实机端到端联调。
- 没有评测反作弊准确率、强对抗、跨重装关联、生产吞吐或长时间运行。规则只是试运行基线。
- Key Attestation、NDK、多租户、数据库索引、业务幂等、入口挑战 / 限流、留存删除和审计尚未实现，阶段计划见 research.md。

本版只用于独立本地开发联调，未部署生产，也未触碰现有游戏仓库。
