# Android SDK 0.7 · 运行环境线索

0.7 新增运行完整性采集。所有结果都是“观测到的线索”：攻击者可以隐藏，读取失败如实上报，不当作“干净”。服务端按规则加分，后台“设备 → 设备参数 → 运行环境”逐项展示。

## 新增信号

| 信号 | 模块 | 说明 |
|---|---|---|
| `integrityNative.tracerPid` | sdk-secure（NDK） | `/proc/self/status` 的 TracerPid，发现 ptrace 类底层调试器（Java `Debug.isDebuggerConnected` 只覆盖 JDWP） |
| `integrityNative.hooks` | sdk-secure（NDK） | `/proc/self/maps` 中 Frida、Xposed/EdXposed、LSPosed、Substrate、Riru、Zygisk、SandHook 的库痕迹 |
| `integrityNative.hookThreads` | sdk-secure（NDK） | Frida 注入线程名（gum-js-loop、gmain、gdbus 等） |
| `integrityNative.emulator` | sdk-secure（NDK） | qemu / goldfish / ranchu / VirtualBox / 夜神 / 雷电 / MuMu / Genymotion 的系统属性与设备文件 |
| `appDebuggable` | sdk | 应用自身为可调试构建（正式包出现通常意味着被重打包） |
| `adbEnabled`、`developerOptions` | sdk | `Settings.Global`，无需权限 |
| `httpProxy` | sdk | 当前网络或全局 HTTP 代理（抓包工具常用） |
| `accessibilityServices` | sdk | 已开启的无障碍服务数量，只上报数量，不上报应用名 |
| `appSandbox` | sdk | Android 用户 ID 与应用数据目录；数据目录不属于本应用时视为多开 / 虚拟容器 |

全部读取均无需新增权限；没有扫描已安装应用列表。

## 服务端规则（试行分值）

| 规则 | 分值 | 条件 |
|---|---|---|
| 底层调试器附加 | 25 | TracerPid > 0 |
| Hook 框架痕迹 | 40 | maps 中发现框架库 |
| Hook 注入线程 | 30 | 未发现库但发现注入线程 |
| 模拟器特征 | 35 | 任一模拟器属性或文件 |
| x86 运行环境 | 15 | 底层 `uname` 为 x86_64 / i686（Chromebook 也会命中，分值较低） |
| 缺少加速度传感器 | 15 | 传感器列表中没有加速度计 |
| 应用为可调试版本 | 30 | `FLAG_DEBUGGABLE` |
| 多开 / 虚拟容器 | 30 | 数据目录不属于本应用 |
| 分身或多用户空间 | 5 | 用户 ID 非 0（工作资料、手机分身） |
| HTTP 代理 | 10 | 存在代理 |
| USB 调试已开启 | 5 | `ADB_ENABLED` |

分值未经校准，正式启用前应在目标机型上观察误报。

## 0.7.2 新增

| 信号 | 说明 |
|---|---|
| `riskApps` | 固定名单（Root/Hook 管理器、多开虚拟机、自动化脚本、虚拟定位、抓包、改内存、远程控制，共 42 个包名）逐个查询是否安装。名单写入 sdk-secure 的 `<queries>`，不申请 `QUERY_ALL_PACKAGES`，不读取应用列表。随机包名的隐藏版本（隐藏 Magisk、寄生式 LSPosed 管理器）看不到 |
| `accessibility` | `AccessibilityManager` 返回的已开启服务（包名、是否系统应用、能否模拟手势）和 `Settings.Secure` 中登记的服务包名；无障碍开启但没有任何登记服务，说明有 UiAutomation 客户端（UiAutomator、Appium） |
| `automation` | `ActivityManager.isUserAMonkey()`；`TouchAudit` 统计的按下事件中，压力与面积均为 1 且无接触椭圆的模拟点击数（input 命令、Instrumentation、无障碍手势都是这种形态）。需在 Activity 的 `dispatchTouchEvent` 调用 `TouchAudit.record` |
| `javaHooks` | 系统与应用类加载器能否加载 XposedBridge、Pine、SandHook 等类；当前调用栈中是否有 Hook 框架帧 |
| `integrityNative.injectedCode` | 可执行映射来自 `/data/local/tmp`、memfd（排除 ART JIT 缓存）或 `/data` 下已删除文件 |
| `integrityNative.ports` | 对 127.0.0.1 全部 65535 个 TCP 端口做连接扫描（Android 10 起应用读不到 `/proc/net`），8 线程，约 2 秒；对开放端口并发发送 Frida 的 D-Bus AUTH 与 HTTP 探测，识别任意端口上的 frida-server；再检查这些端口能否经本机网卡地址访问（对局域网开放） |

扫描为降低开销直接发起 `socket`/`connect` 系统调用，跳过 bionic 对每次连接的 netd 打标 IPC；回环连接不需要网络标记。

## 0.7.3 修正

登记新密钥时默认带 `setAttestationChallenge` 生成。部分设备的 Keystore 无法生成证明（例如出厂证明密钥损坏，`generateKeyPair` 抛 `ProviderException`），以前会导致登记和后续所有上报失败。现在：

- 挑战 `hardwareRequired = false`：删除失败残留的别名，不带证明重新生成密钥，并在登记包中附带原始错误 `attestationError`。平台记为 `signature_only`、`issue = keystore_failed`，verdict 中 `app.attestation = none`。
- 挑战 `hardwareRequired = true`：照旧直接抛出，不降级；平台也会拒绝带 `attestationError` 的登记。

## 0.7.4 新增

| 信号 | 说明 |
|---|---|
| `lan` | 第一个 Wi-Fi / 以太网网络，或网卡名为 `eth*` / `wlan*` 的网络（箱式、云手机常把 eth0 标成移动网络），VPN 之下的物理网络也能取到：IPv4 地址与前缀、默认网关、IPv4 DNS、DHCP 服务器（API 30+）、网卡名、系统标记的网络类型。只用 `ACCESS_NETWORK_STATE` |
| `lanProbe` | 挑战带 `lanTargets` 时，`SecureReporter.upload` 在签名前探测这些地址：只接受点分十进制且位于本机网段内的地址（不经 DNS），16 线程，每项 300 ms，总预算 3 秒。`InetAddress.isReachable`（无特权 ICMP，失败时 TCP 7 端口）判断可达，TCP 连接 5555 判断 ADB 无线调试；同时探测网关 |

## 0.7.6 新增

| 信号 | 说明 |
|---|---|
| `drmId` | `MediaDrm(Widevine).getPropertyByteArray(PROPERTY_DEVICE_UNIQUE_ID)` 的 SHA-256，不需要权限，原值不离开手机；与 ANDROID_ID 同受 `includeAndroidId` 控制。平台用它在重装后找回原 deviceId |

## 构建

Linux 可直接构建：`scripts/build-native.sh <NDK 23.1.7779620> <xollvm v0.12.0 Linux LLVM22>`，产物与 Windows 脚本一致。
