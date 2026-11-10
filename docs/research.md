# 设备风控研究与首版范围

核对日期：2026-11-10。本文区分厂商公开声明、平台 API 能力与我们的实现决策。产品宣传不能作为准确率实测，也不能推出其底层一定采集了某个字段。

## 1. 数美公开能力矩阵

| 能力 | 官方公开证据 | 我们需要的功能 | 实现边界 / 阶段 |
| --- | --- | --- | --- |
| 设备关联标识 | [产品概述](https://help.ishumei.com/docs/tw/sdk/overview/)列有设备标识 | 应用内设备画像、历史关联 | P0 使用本地随机 ID；跨重装、跨应用关联需后续验证 |
| 篡改与伪造识别 | [产品页](https://www.ishumei.com/m/product/bs-post-sdk.html)描述多维交叉分析 | 属性一致性、变化轨迹 | P1；没有厂商设备库，不能承诺相同效果 |
| 模拟 / 虚拟环境 | 同上，包含模拟器和云手机 | 环境线索组合 | P0 仅简单模拟环境线索；真机农场不能据此排除 |
| 积分墙风险 | 同上，声明可识别相关工具风险 | 工具与异常任务行为关联 | P2；包可见性、样本与商店政策制约，不能保证全量扫描 |
| 运行环境 | 同上，列举代理、调试等 | 调试与网络状态 | P0 调试 / VPN 线索，VPN 本身不代表作弊 |
| 行为与聚集风险 | [产品概述](https://help.ishumei.com/docs/tw/sdk/overview/)含行为风险 | 服务端账号与事件频次 | P0 可解释规则；P2 样本驱动关联模型 |
| 多应用接入 | [Android 文档](https://help.ishumei.com/docs/tw/sdk/android/developDoc/)的组织 / 应用参数 | 应用、凭据隔离 | P0 单工作空间、多应用；多租户 P2 |
| 采集控制与接入时机 | 同上，要求同意后初始化并提供禁采配置 | 宿主显式调用、字段开关 | P0 不自动初始化；敏感模块后续逐项接入 |
| 标识与报告区别 | 同上，v3 的 boxId / boxData 不是可直接使用的明文设备标识 | 上报报告与画像键分离 | P0 reportId 仅是报告引用；installationId 只是关联线索 |
| 触控 / 内存子模块 | 同上，文档有按需提供的子包 | 必要场景微行为、篡改研究 | P2，先收集必要的业务行为 |
| 实时业务判断 | [渠道 API 简介](https://help.ishumei.com/docs/tw/channel/api/)描述业务事件实时风险与控制台 | 业务端调用评估、查看事件 | P0 服务端事件评估与工作台 |
| 群体风险情报 | 同上，描述全局画像与关联 | 应用内关联、反馈闭环 | P2 从自有合法样本建立；不等同厂商全局情报 |

以上是能力存在性确认。没有公开验证：具体检测覆盖清单、准确率、误判率、稳定 ID 的跨重装表现、国内外 ROM 差异、强对抗效果。不能把厂商宣传直接写成本项目验收指标。

需商务确认：正式接口与返回标签、配额 / 价格 / SLA、SDK 版本及包体和架构、微行为子包授权、独立风险画像产品与 SDK 的套餐关系、设备 ID 解密授权、私有化数据边界、区域数据流与保留期限、Key Attestation 是否参与其产品及如何验真。渠道 API 页面明确提示以正式对接文档为准。当前仅研究公开资料，没有接入数美 SDK 或服务。

## 2. Android 信号逐项核实

| 信号 | 普通应用能力与约束 | 本项目决策 | 官方来源 |
| --- | --- | --- | --- |
| 基站 / 小区 | getAllCellInfo 需要精确位置权限及蜂窝硬件；可能为空、缓存、限频，更新不保证实时；要记录时间戳与 unavailable 哨兵 | P1 可选模块；权限、系统位置状态及 ROM 实机矩阵验证后启用；不把基站等同 GPS | [TelephonyManager](https://developer.android.com/reference/android/telephony/TelephonyManager#getAllCellInfo()) |
| 蜂窝信号 | 无线制式、订阅、硬件与对应 API 条件不同，不能假设任意设备均有实时 dBm；CellInfo 的测量需与样本时间一并解释 | P1 和基站模块一起处理，不以缺失判作弊 | [TelephonyManager](https://developer.android.com/reference/android/telephony/TelephonyManager) |
| 运营商 | 网络运营商与 SIM 运营商是不同概念；漫游、多卡、无服务会影响结果；MCC/MNC 不是实名信息 | P0 仅默认网络运营商代码，空值显式 unavailable；多卡后续独立设计 | [TelephonyManager](https://developer.android.com/reference/android/telephony/TelephonyManager#getNetworkOperator()) |
| Wi-Fi / BSSID / RSSI | BSSID 是当前 AP 的无线接口标识；位置相关字段有权限约束，缺权限可返回占位地址；没有连接时可为空；扫描接口还存在限频 | P1 按版本实现；权限不足不存占位地址当真实指纹；RSSI 与扫描分开 | [WifiInfo](https://developer.android.com/reference/android/net/wifi/WifiInfo) |
| Android 13+ Wi-Fi 权限 | 部分附近 Wi-Fi 操作使用 NEARBY_WIFI_DEVICES；部分扫描接口仍需精确位置；不能用 neverForLocation 声明掩盖实际位置推断用途 | 随具体 API 明确权限，不照抄旧 SDK 的权限列表 | [Wi-Fi 权限](https://developer.android.com/develop/connectivity/wifi/wifi-permissions) |
| 定位 | 精确 / 大致权限、前后台与用户设置共同决定；缓存位置可能过期或为空；mock 标记也不是可信证明 | P1 按需前台采集，附时间与精度；默认关闭 | [LocationManager](https://developer.android.com/reference/android/location/LocationManager#getLastKnownLocation(java.lang.String)) |
| IP | SDK 看到的接口地址与服务端看到的出口地址不同；NAT、VPN、代理使 IP 不等于设备；GeoIP/ASN 是外部数据服务 | P0 保存连接对端 IP；忽略用户伪造的 X-Forwarded-For。业务事件另收业务后端确认的 clientIp；P1 再配置可信代理 | [Android 网络连接](https://developer.android.com/develop/connectivity) |
| 路由 MAC | BSSID 不是路由器 WAN/LAN MAC，也不是手机 MAC；普通 App 没有跨网络获取“路由器真实 MAC”的通用保证 | 不采集，不通过读取 ARP 表声称能稳定识别路由硬件 | [WifiInfo](https://developer.android.com/reference/android/net/wifi/WifiInfo)、[Android 10 隐私变化](https://developer.android.com/about/versions/10/privacy/changes) |
| 手机 Wi-Fi MAC | 非重置硬件标识与随机化机制限制获取，不能作稳定设备主键 | 不采集 | [Android 10 隐私变化](https://developer.android.com/about/versions/10/privacy/changes) |
| 蓝牙 MAC | 本机真实地址涉及 LOCAL_MAC_ADDRESS 等额外限制；Android 12+ 通信权限不自动授予读取本机真实 MAC 的能力；周边设备地址可随机化 | 不采集本机 MAC；不把周边 BLE 地址当设备身份 | [BluetoothAdapter.getAddress](https://developer.android.com/reference/android/bluetooth/BluetoothAdapter#getAddress()) |
| 应用列表 | target 30+ 默认过滤，queries 只扩大明确范围；QUERY_ALL_PACKAGES 还受商店用途限制；不可把可见子集称全量列表 | P0 不收；P1 若确有必要，只设计明确包范围与可见性状态 | [包可见性](https://developer.android.com/training/package-visibility) |
| ANDROID_ID | Android 8+ 按签名、用户与设备范围；重置和签名变化等会影响，不是通用跨应用硬件 ID | P0 可选，默认由 Options 明确关闭；不用于认证 | [Settings.Secure](https://developer.android.com/reference/android/provider/Settings.Secure#ANDROID_ID) |
| IMEI / 序列号等 | 新系统对非重置标识有特权 / 角色等限制，普通应用不可保证获得 | 不索要 READ_PHONE_STATE 以承诺获取所有标识 | [标识符指南](https://developer.android.com/identity/user-data-ids)、[Android 10](https://developer.android.com/about/versions/10/privacy/changes) |
| OAID / 广告 ID / App Set ID | 各自用途、提供方、服务依赖、重置机制不同；不是通用防篡改根 | P1 按具体发行环境讨论，首版无第三方标识依赖 | [标识符指南](https://developer.android.com/identity/user-data-ids) |
| Key Attestation | API 24+，硬件支持和设备证书链有差异。需要独立可信服务验证完整链、根、撤销状态及安全级别；不能信任 App 自报“验证成功” | P1 必须做服务端一次性随机 challenge、有效期、包签名绑定、证书扩展与启动状态解析及私钥持有证明；首版标记 disabled | [官方硬件证明指南](https://developer.android.com/privacy-and-security/security-key-attestation) |
| NDK | 原生代码仍运行在应用沙箱与 UID 权限下；可增加独立采样和 native 一致性检查，不能绕过权限 | 当前 Java API 足够，不引入空壳 so；P1 有具体研究与验证收益再加入 | [Android 应用沙箱](https://source.android.com/docs/security/app-sandbox) |

数美 Android 文档列出位置 / Wi-Fi / 外部存储权限，不能据此推导新系统只授粗略位置就能读取全部基站，或外部存储可维护跨应用稳定 ID。本项目按实际系统 API 和 targetSdk 复核。

## 3. 信任模型

1. 所有客户端字段（包括 package、安装 ID、VPN、调试、系统属性）都可能被伪造。多源一致只提高置信度，原始指纹不是可信证明。
2. 客户端只上报报告，返回 reportId。公开 appId 不是身份凭据。业务后端验证自己的用户会话，使用私有应用密钥提交风险事件，传入自身确认的账号与用户 IP。密钥绝不嵌入 APK。
3. 评估结果包含规则版本、分数、原因与建议。低分代表当前规则未命中；缺失权限不直接计作风险，分值不是概率。
4. 目前规则仅使用自报调试 / 模拟 / VPN / 包名和业务端事件频次、账号数量。多个账号是同一客户端随机 ID 的关联，不可称物理设备真实关联；重装、清数据、备份恢复和改 ID 会影响关联。
5. Attestation 只能支持对密钥及证明时设备状态的有限推断，不能证明所有后续请求、所有业务行为均真实。证书根轮换与撤销需持续维护。

## 4. 阶段与验收

| 阶段 | 交付 | 验收 |
| --- | --- | --- |
| P0 当前首版 | Java SDK / Kotlin 调用 API、基础信号与禁采状态、独立 Go API、多应用 / 密钥轮换、事件规则与统计、Vue 后台 | AAR 构建、Go 测试、Vue 构建；权限缺失不伪造数据；跨应用报告不可混用、旧密钥失效 |
| P1 真实设备试点 | 可选基站 / Wi-Fi / 定位、签名信号、native 研究、服务端完整 Attestation、持久化索引与业务事件幂等、可信代理 | Android 版本 / ROM / 无 SIM / 多卡 / 拒绝权限 / mock / 重放 / 证书异常矩阵；保留样本与误报复盘 |
| P2 风险运营 | 画像变化、账号 / 网络关联、标签反馈、规则配置与审计、多租户与权限、保留和删除机制 | 与业务确认样本真值，离线精确率 / 召回率、分场景误报；影子模式评估后再启用阻断 |
| P3 商业运行 | 容量、容灾、密钥托管、发布与回滚、持续攻防与隐私评审 | 压测与恢复演练、权限及市场审核、监控告警、明确 SLA |

首版采用 Go 标准库，每条数据独立 JSON 文件，原子发布，便于直接排错和本地查看。适合开发联调；每次分析扫描数据，规则在并发请求下采用各自读到的历史快照，不能提供精确限额扣减或强事务频控。正式试点前迁移有索引的数据库。没有自动拦截、全局黑产库、跨应用硬件主键、证书验真或商业可用性承诺。
