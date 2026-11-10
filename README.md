# Device Trust Platform

独立的 Android 设备风险 SDK 与风控平台。当前阶段先研究数美科技公开的设备指纹与反欺诈产品能力，形成有来源的功能对照与产品范围，再开发 SDK、服务端和管理后台。

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
