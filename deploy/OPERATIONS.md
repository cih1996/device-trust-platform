# DeviceTrust 0.4.1 线上交付

上线日期：2026-11-11（Asia/Shanghai）。

- 官网：https://devicetrust.lingzidev.com
- 工作台：https://devicetrust-console.lingzidev.com
- API：https://devicetrust-sdk-api.lingzidev.com
- 接入手册：https://devicetrust.lingzidev.com/docs/
- SDK：https://devicetrust.lingzidev.com/downloads/devicetrust-sdk-0.4.1.zip
- APK：https://devicetrust.lingzidev.com/downloads/devicetrust-0.4.1.apk

登录凭据另存于本次交付的私密 JSON：adminToken 用于后台登录；demoPassword 用于体验 App；App 的账号填自选测试名称。为真实业务在工作台创建独立应用，应用密钥只保留在业务服务端。

## 独立部署

Ubuntu x86_64；目录 /opt/devicetrust/releases/0.4.1，current 为软链接。运行数据 /var/lib/devicetrust，权限 0700；私密凭据不在官网目录。专用低权限用户 devicetrust，二进制与静态文件归 root 所有。

平台仅监听 127.0.0.1:18191，体验业务仅监听 127.0.0.1:18192。Nginx 独立配置 /etc/nginx/sites-available/devicetrust，三个域名分别承载官网、后台、SDK/业务 API。API 域名不暴露管理 /api/ 路由，后台域名不暴露 /v1/。Nginx 覆盖客户端 IP 头，应用只在显式启用时信任来自回环地址的代理。入口限制请求体与请求频率，超额返回 429。

服务设为开机启动：devicetrust-platform、devicetrust-demo。trust.timer 每日调用官方刷新任务。查看：

```sh
systemctl status devicetrust-platform devicetrust-demo
journalctl -u devicetrust-platform -u devicetrust-demo -n 100
systemctl status devicetrust-trust.timer devicetrust-trust.service
nginx -t
```

本次只新增 DeviceTrust 配置，原有 Nginx 站点配置 SHA-256 前后相同，已有站点回归访问 200。未停止其他业务服务。Nginx 原配置快照在 /opt/devicetrust/backups/nginx-before-0.4.1.tar.gz。

## 当前验证与待办

公网 HTTPS 全链路测试通过：签名注册、加密报告、篡改 403、相同报告重试去重、暂缓、批准、执行与回执。确认真实公网客户端 IP 入库，重试只有一条演示业务效果。官网、文档、后台与健康检查均 200。APK 发布构建、Lint、签名与 16 KB zipalign 通过。

服务器目前无法连接 android.googleapis.com 和 developer.android.com，IPv4 请求也超时。初始证明根与撤销状态是从本机直接访问 Google 官方接口取得后上传，未替换为其他来源。每日刷新任务仍使用官方接口，若出口未修复会明确失败。快照超过 24 小时后硬件证明拒绝请求；签名模式不依赖该快照。

因此体验应用硬件证明保持关闭。启用前需要提供可访问官方接口的网络出口，并完成 TEE/StrongBox/OEM 真机兼容验收。APK 真机运行、实际采集权限矩阵仍未验收；演示支付只写隔离账本，正式支付须接入业务系统和渠道回调。文件扫描存储、识别准确率和容量仍需试点评估。

## 证书与更新

三张证书均于 2027-02-09 00:59:59（Asia/Shanghai）到期。提前申请续期证书并替换 /etc/nginx/ssl/devicetrust 下对应的 fullchain.pem 与 privkey.pem，私钥权限 0600；nginx -t 通过后 reload。

API 证书公钥若变化，须先发布包含新旧 SPKI pin 的 App/业务配置，再切换证书。当前 pin：AU7LGDAFJUgs4HkcpRf1O7/6OU9avVMvle6wTsAK5m4=。证书续期使用同一公钥时无需更改 pin。APK 与原 0.4 使用同一签名证书，可覆盖升级。

如需撤下本产品，只停用 devicetrust-* 三个服务/定时器并移除 sites-enabled/devicetrust 软链接，nginx -t 后 reload；保留数据目录供恢复。不要停止或覆盖其他站点。

## 发布与备份轮换

每次发布前在服务器上执行 `sudo /opt/devicetrust/bin/backup-data.sh <版本>`（源文件 `deploy/backup-data.sh`）：

- 把 `/var/lib/devicetrust/platform` 打包为 `/opt/devicetrust/backups/data-before-<版本>-<UTC 时间>.tar.gz`，并校验归档可读。
- 在 `/opt/devicetrust/backups/backup.log` 追加一行：时间、文件名、大小、SHA-256。
- 只保留最新 10 份数据备份；`releases/` 按版本号只保留最新 10 个，`current` 指向的版本不会删除。每次删除也记入 backup.log。
- Nginx 快照和 service 文件备份不参与轮换。

恢复数据：停止 devicetrust-platform，把 backup.log 中对应的归档解压到 `/var/lib/devicetrust`，再启动服务。

## 基站库（OpenCelliD）

- 数据：`/var/lib/devicetrust/cells/460.csv.gz`，上一版保留为 `460.prev.csv.gz`。服务以 `-cell-db` 参数启动时加载，文件损坏会导致启动失败。
- 令牌：`/etc/devicetrust/opencellid.token`（root:devicetrust 0640），不要提交到仓库。
- 更新：`devicetrust-cells.timer` 每月运行 `cell-sync`，下载后用平台同一解析器校验；条数不足上一版一半时保留上一版。成功后重启平台服务以加载新表。
- 手动更新：`sudo systemctl start devicetrust-cells.service`；查看结果：`journalctl -u devicetrust-cells`。
