# 小范围外部使用

外部入口：https://221.216.94.213:18443/ 。仅使用 HTTPS 设备证书认证，不需要账号密码。路由器只映射 TCP18443。
内部入口：https://192.168.1.71/chaoxing/；使用与访问记录：https://192.168.1.71/chaoxing/records.html，仅局域网可读。

设备证书默认有效期90天，Windows 私钥在使用者电脑生成且不可导出。设备名称用于记录归属；外部只能读取该设备的任务与报告。

## 接入使用者

1. 将 `deploy/chaoxing-helper/New-AigcDeviceRequest.ps1` 发给使用者，在其电脑运行：

```powershell
powershell -ExecutionPolicy Bypass -File .\New-AigcDeviceRequest.ps1 -User alice
```

使用者仅发回 `%USERPROFILE%\AigcDevice\alice.csr`。

2. 管理员在本仓库运行：

```powershell
python scripts/aigc-access.py grant alice C:\收到的文件\alice.csr
```

输出目录包含设备证书、两份根证书与 `connection.txt`；不再生成密码。管理脚本沿用本机 serverctl 连接。

3. 将该目录和 `Install-AigcDevice.ps1` 发给对应使用者，在生成 CSR 的电脑安装：

```powershell
powershell -ExecutionPolicy Bypass -File .\Install-AigcDevice.ps1 -PackageDirectory C:\收到的证书目录
```

使用 Chrome/Edge 打开入口，选择本人设备证书即可。已有设备证书继续有效，无需重装。

## 记录与撤销

内网页面分为检测记录、访问记录。检测记录包含设备名称、IP、文档、时间、状态与结果；访问记录保留最近10000条页面访问、提交、报告下载及错误，不保存正文、查询参数或认证秘密。成功的资源加载和进度轮询不重复写入页面记录。
外部完整 HTTP 访问日志位于服务器 Caddy `data/aigc-logs/access.jsonl`，记录设备名、来源IP、路径与状态，单文件5MiB、最多3份轮转文件；TLS握手失败见Caddy运行日志。日志不会开放给外部。

```powershell
python scripts/aigc-access.py list
python scripts/aigc-access.py revoke alice
```

撤销移除设备白名单，后端逐请求核对有效期，已有连接也不能继续提交或读取。换设备先撤销，再用本人新CSR签发，使用 `--out` 指定新交付目录；不同人不要复用设备名称。

## 服务器维护

`/home/docker/services/caddy/public-access.py` 管理签发、撤销和配置；私有CA位于 `aigc-access/private/`，公开设备白名单位于 `aigc-access/public/clients/*.pem`。
运行配置为 `config/managed.json`，修改生成脚本后执行 `render --reload`。Go的网关令牌与有效设备表不公开。IPv4/IPv6非LAN入站及Docker转发限制继续只允许AIGC18443。
维护前保留配置与数据库恢复材料，修改后更新serverctl基线；私钥、浏览器Cookie、文档与报告不入Git。
