# 知衡 0.4 使用说明

Windows x64 后台配套 Android 7.0+ 发布签名 APK，适用于独立测试与业务接入联调。注册、奖励和支付效果存入本地演示账本，演示支付不连接支付机构。

## 启动

1. 解压后台包到固定目录，PowerShell 执行 ./Start-Demo.ps1。首次生成独立应用密钥、管理口令和演示密码；启动时显示后两项。官方证明根与撤销状态从 Google HTTPS 下载，下载失败直接报错。
2. 浏览器打开 http://127.0.0.1:8099，输入管理口令。此 HTTP 入口仅限本机，设备与业务 API 使用 9443/9444 的 HTTPS。
3. 安装配套 APK。手机开启 USB 调试并连接本机，用 Android platform-tools 执行：

```powershell
adb reverse tcp:9443 tcp:9443
adb reverse tcp:9444 tcp:9444
```

4. App 保持 https://localhost:9444，填写测试账号名和演示密码，同意采集后登录。位置单独可选。尝试注册、奖励或支付。
5. 等待核验/审核时，后台风险事件详情选择批准或拒绝；App 点“查询并继续”。后台展示执行结果。
6. 停止执行 ./Stop-Demo.ps1；关闭启动窗口不会停止服务。再次启动保留既有数据。

原始上游错误在 logs/*.err.log。体验默认执行模式、强制签名、硬件证明关闭；启用硬件证明后不支持的设备会被拒绝。管理口令在 data/admin-token；演示密码与私有应用密钥在 data/demo.json。不要分享运行 data/ 和 logs/。移动已初始化目录需同步更改 demo.json 的 CA 与 data 绝对路径，建议放在当前用户私有目录。

## 接入真实业务

config 中的 localhost 测试 TLS 叶证书与私钥配套 APK 固定值，证书有效期 90 天，截止以证书为准。测试 CA 私钥没有保存或交付，APK 签名私钥没有打入交付包。

正式接入需配置自己的域名证书与备用 SPKI、正式 APK 签名后重新构建；用实际账号认证替换演示登录，以业务数据库幂等提交替换演示账本，接入核验/支付回调验签与对账。硬件校验配置正式签名摘要后，应完成目标真机矩阵验收。

至少每日执行 trust-sync.exe -out data/trust 刷新撤销状态，根更新后重启 server。文件扫描存储尚未做生产容量、入口配额和模型误杀率评测，应先隔离试点。

## 构建

Go1.26+；前端 npm ci / test / run check / run build；Android JDK17、Gradle8.11.1、AGP8.9.2、SDK35。基础与 secure 模块无第三方运行库，RootBeer0.1.2 可选。

下载 xollvm v0.12.0 Windows LLVM22 官方工具包，SHA-256：c8cc537a0837529ec009e85ff838026f78047b6465c580ffc373356600907ef4。使用 NDK23.1.7779620 执行 scripts/build-native.ps1 -NdkPath <NDK> -ObfuscatorPath <xollvm>，生成四 ABI。工具链不随源码分发。

发布设置 DTP_RELEASE_KEYSTORE、DTP_STORE_PASSWORD、DTP_DEMO_PIN；测试 CA 放 sample/res/raw/development_ca.pem，执行 gradle -p android :sample:assembleRelease :sample:lintRelease :sdk-secure:assembleRelease :sdk-secure:lintRelease。原生库缺失直接失败。SDK AAR 的使用需同时引入 sdk、sdk-secure；环境模块可选，rootbeer 适配需额外引入 com.scottyab:rootbeer-lib:0.1.2。

## 验证范围

Go 全部测试与 vet、前端 3 测试/模板编译/构建通过。混淆 JNI 在主机 JDK 实际运行，产出帧由 Go 解密验签；篡改、错误 salt、错误 pin 被拒绝。四 ABI 交叉编译、R8、APK v2 签名和 zipalign 16 KB 通过，自有 secure SO LOAD 段 16 KB 对齐。Gradle strip 提示对应原生脚本已经手动去符号的 SO。

sample Lint 无问题；secure Lint 0 errors/1 warning，提示自定义 X509TrustManager 的风险。实现调用平台扩展验证完整证书链后核验固定值；保留该提示，需继续真机握手与抓包测试。

实际启动交付服务的 TLS 注册、二进制上报、去重、暂缓、审批、执行、重试通过。故障注入测试确认回执 503 时保留错误，重试补回执且账本仅一条。浏览器登录、已完成事件展示、保护配置保存通过，未捕获 error/warn。

尚未完成 APK 真机运行、OEM/TEE/StrongBox 证书兼容、RootBeer/权限矩阵、16 KB 页面设备运行、真实支付渠道、压力与识别准确率验收。硬件证明测试为隔离合成证书，不能代表真实设备认证结果。
