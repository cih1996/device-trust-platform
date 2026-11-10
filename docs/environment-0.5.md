# 0.5 资源验真与业务绑定

## 业务接口必须接入执行关口

SDK 采集后返回的 deviceId 是按应用、Keystore 公钥登记的不透明标识，不能充当登录凭据或风控通行证。即使参数相同，同型号设备也分别持有密钥与设备 ID。更新 App 后 ID 保持；卸载、清除 Keystore 或重新注册可能改变 ID，当前不声称永久跨重装识别。

业务服务器认证当前账号后，将完整业务意图规范化：HTTP 方法、路径、账号、订单、金额、币种、收款对象、优惠对象等。计算 SHA-256 requestHash，将规范化结果与操作编号保存在业务数据库。摘要由业务服务器计算，不能采用客户端声称的值。

调用 /v1/challenges 时传 context.requestHash；/v1/events 使用完全相同的 context；/v1/decisions/claim 的顶层 requestHash 必须与挑战和事件一致。修改参数、账号或接口会得到不同摘要，不能领取原授权。原始请求重复必须在业务数据库按唯一 operationId 原子去重。风控服务无法替一个未接入这些校验的第三方业务接口消除重放。

Go 业务接入包在 pkg/businessguard；Authorize 同时校验绑定、执行模式和到期时间。应用密钥与 permit.token 只在服务端流转。取得 permit 后，在业务数据库事务中按 operationId 唯一约束提交效果；外部支付渠道使用同一幂等键。已提交业务恢复时读取原记录补回执，不重新支付。demo-business 是完整的隔离账本示例。

新建应用默认要求请求摘要。已有应用升级后需在“接入保护”开启业务请求绑定，完成上述业务改造。服务端返回的挑战新增 hardwareRequired；SDK 根据明确策略生成普通签名密钥或带证明的密钥，普通模式不再发起证明证书请求。强制硬件模式失败直接拒绝，不自动降级。普通签名密钥持续复用，直至吊销或本地删除；验证有效期用于硬件证明，避免普通设备 ID 按日变化。

## 采集与比对

ResourceCollector 采集 Java/NDK 双通道的品牌、制造商、机型、设备代号、硬件平台、内存、核心数，以及 ABI、SoC 与指定传感器线索。NDK 直接读取 uname、sysconf、/proc/meminfo、/proc/cpuinfo 和公开系统属性；不读取 CPU 序列号或全量已安装应用。ResourceCollector 模块包含在 sdk-secure AAR，需在用户同意采集后显式调用。

指定系统应用只读取包名、版本、系统标记和当前 APK 证书 SHA-256。目标列表已声明 queries，不使用 QUERY_ALL_PACKAGES。应用不存在、不可见、权限不足分别保留状态；不能因未检出厂商应用直接断定是假机。基线按品牌/型号/固件范围配置，允许多个签名摘要，证书轮换需更新基线。

Java 与 native 内存比较保留系统预留和采样容差；核心数只检查运行可见数量超过底层配置数的矛盾，不将节能或 cpuset 限制判成异常。SKU 基线允许内存变体。当前 MI 8 SE 规格引用小米官方页，签名基线来自客户指定官方 ROM 的两路实验设备，属于实验基线而非独立厂商签名背书。

参数与包名一致性提高发现单点改机的机会，不是硬件级真实性证明。控制整个系统的攻击者仍可能同步伪造多路结果。

## 网络资源

IP 以服务端实际连接观测值为准；Nginx 覆盖代理头。IPv4/IPv6 离线 ip2region 快照已接入，版本、来源与 SHA-256 保存于 ip-manifest.json。IP 归属是运营商出口粒度，不能当作精确用户位置。

GPS 使用 WGS84 坐标、精度、样本年龄与模拟标记；基站增加无线制式、MCC/MNC、区域码、Cell ID、驻网状态与年龄。只有新鲜且精度合适的样本参与比较。移动网络出口、VPN、边界附近、缺失坐标库、过期或低精度信息返回“信息不足/暂不可比”，不按作弊计分。

Natural Earth 公有领域的 50m 省级边界只用于粗区域比较，不展示地图。当前覆盖源数据中 31 个中国内地区域。普通固定网络的 IP/GPS 跨省判断保留 100km 边界缓冲；移动网络和 VPN 不据此硬判跨省作弊。

基站/GPS 距离引擎已支持带来源、更新时间和覆盖半径的本地基站目录。当前没有可信基站坐标数据，因此实际部署不会虚构基站位置；OpenCellID 公开接口仍需要账号 API key，并按 CC BY-SA 4.0 授权。后续可导入授权数据到 catalog.cells，无需更改客户端协议。

资源目录由 -resource-catalog 指定；IP 库由 -ip-v4/-ip-v6 指定；更新后重启本产品平台服务加载。/api/resources 可查看覆盖摘要。后台事件与画像显示每项“已比对/有矛盾/信息不足/暂不可比/采集出错”及依据。

## 受测环境与限制

两路 MI 8 SE / Android10 / ARM64 环境已实际运行 R8 发布版与四架构混淆库中的 ARM64 库，执行设备登记、Keystore 签名、加密上报与业务重试，重复运行设备 ID 保持。前置测试发现设备 DNS 不可用；完整公网协议测试借助仅连接本项目 API 的本机 CONNECT 代理，保留原域名、系统证书校验和 SPKI，没有关闭 TLS 验证。这不代表设备自身网络已通过直连验收。

无定位权限和授权后无基站/GPS缓存两种路径均实测。用户当前没有可联网、带 SIM/GPS 的测试设备，因此真实 IP—基站—GPS 交叉验证仍待具备样本；距离、时效、权限和参数篡改用隔离合成用例验证，不计为真实定位准确率。

硬件证明测试在两路设备均返回 ProviderException / KeyStoreException -10003，未生成证明链。没有将资源一致性、ROM 包名或普通签名标记为 TEE 证明通过。硬件真实性验收仍需能生成可验证证明的目标机型或国内厂商证明服务。

来源：[小米8 SE 官方规格](https://www.mi.com/mi8i/specs)、[Android 包可见性](https://developer.android.com/training/package-visibility)、[ip2region](https://github.com/lionsoul2014/ip2region)、[Natural Earth 授权](https://www.naturalearthdata.com/about/terms-of-use/)、[OpenCellID 接口](https://docs.opencellid.org/docs/api/overview)。
