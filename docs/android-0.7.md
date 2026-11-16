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

## 构建

Linux 可直接构建：`scripts/build-native.sh <NDK 23.1.7779620> <xollvm v0.12.0 Linux LLVM22>`，产物与 Windows 脚本一致。
