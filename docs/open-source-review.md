# GitHub 采集组件评估

核对日期：2026-11-10。本轮目标是复用少量明确的采集能力；风险关联、规则与业务判断仍由本项目服务端完成。

| 项目 | 已查来源 / 授权信息 | 适合复用部分 | 本轮决定 |
| --- | --- | --- | --- |
| [scottyab/rootbeer](https://github.com/scottyab/rootbeer) | [0.1.2 README](https://github.com/scottyab/rootbeer/blob/0.1.2/README.md)、[源码](https://github.com/scottyab/rootbeer/blob/0.1.2/rootbeerlib/src/main/java/com/scottyab/rootbeer/RootBeer.java)、[LICENSE](https://github.com/scottyab/rootbeer/blob/0.1.2/LICENSE)；Apache-2.0，Maven Central | 测试签名标签、可见 su / magisk 文件、原生路径检查 | 已接入可选 sdk-rootbeer，固定版本 0.1.2 |
| [nisrulz/easydeviceinfo](https://github.com/nisrulz/easydeviceinfo) | [README](https://github.com/nisrulz/easydeviceinfo/blob/master/README.md)、仓库 Apache-2.0 标识；设备类别分模块，文档仍使用 JCenter / Bintray | 电池、屏幕、传感器等字段分类 | 参考能力划分；当前基础字段已有平台 API 实现，未引入该依赖 |
| [android/keyattestation](https://github.com/android/keyattestation) | [README](https://github.com/android/keyattestation/blob/main/README.md)、[构建配置](https://github.com/android/keyattestation/blob/main/build.gradle.kts)；Apache-2.0；JVM / Kotlin 实现，含 ASN.1 / CBOR 等相关依赖 | 可信服务端证书链验证与测试案例 | 后续优先评估；当前 Go 后台没有因此增加 JVM 运行时，也没有实现完整验真 |
| [AmIUniqueTools/AmIUniqueApp](https://github.com/AmIUniqueTools/AmIUniqueApp) | 仓库标识 AGPL-3.0，研究项目含反射 / shell / ContentProvider 等采集路径 | 研究采集面与系统适配样本 | 本轮不纳入交付依赖，不复制代码 |

没有依据 Star 数决定接入。没有拉取或运行不明 APK，也没有把研究工具的全部采集行为移入 SDK。

## 实际复用：RootBeer 0.1.2

上游对应提交：`de7bc7b4b09c9db148b62b331b4a44076b2eb480`。Gradle 固定 `com.scottyab:rootbeer-lib:0.1.2`，可选模块不打入基础 SDK。上游构建配置无 Android 运行时传递依赖，发布 AAR 包含多架构原生库。

我们的适配层仅调用 `detectTestKeys`、`checkForSuBinary`、`checkForMagiskBinary`，以及在显式加载原生库之后直接调用 `RootBeerNative.checkForRoot`。原生检查使用适配层列出的六个路径。上报的是单项字段和组件版本，不上报汇总“真实设备”结论。

没有调用 `isRooted`、shell 命令、包列表扫描和把异常改成 false 的便捷方法。原生加载失败的 `UnsatisfiedLinkError` 保留原始错误字符串到 error 状态，并输出完整 logcat 堆栈；其他已完成字段仍可上报。上游源码未修改。

`false` 只表示本次方法未发现线索。路径不可见、隐藏 Root、改名工具、定制 ROM 都可能使方法失效。0.2 只记录字段；rules-0.3 在至少两项可用的 su / magisk / native-su 观察为 true 时加 20 分，来源仍标为客户端。Java / native 检查同一路径不构成独立可信证明。Root、测试版系统或 VPN 单独出现均不等于业务作弊。

随交付保留上游版权说明和完整授权文本：`third_party/NOTICE.md`、`third_party/licenses/rootbeer-LICENSE.txt`。发布包附独立上游 AAR；自定义 SDK AAR 不会自动内嵌其他 AAR，接入方必须按 integration.md 同时添加所需文件与依赖。

## 自有采集模块

sdk-environment 使用 Android 公开 API，按 WIFI / CELL / LOCATION 显式开启，无第三方依赖。它与基础 SDK 分离，位置与 Wi-Fi 权限只在选择此模块时进入宿主合并清单。它读取当前连接或系统缓存，不请求主动扫描、定位刷新或持续监听。

Wi-Fi 读取当前 BSSID 与 RSSI，过滤系统脱敏占位 BSSID；基站解析 LTE / GSM / WCDMA / NR 的小区、区域、dBm、注册状态与样本年龄，不支持的制式明确标注；定位保留坐标、精度、年龄与 mock 标记。平台未知哨兵输出 null，未知或异常时间戳不会伪装成新鲜样本。权限缺失、缓存缺失、未启用和原始执行错误分别记录。

仍需真机版本 / ROM / 权限矩阵测试。这里的缓存、mock 标记和文件检查全部是可伪造的观察值，不构成硬件证明。
