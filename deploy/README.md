# 部署

Go 提供 AIGC 网页；现有 server-readme Nginx 提供服务器面板；Caddy 仅反向代理。原 Markdown 仍从 /server-readme/README.md 读取。

部署包包含按需无头 Chromium、私有 CDP 中继、临时登录桌面与主机管理器。真实 Word 提交、检测、报告对应及按需退出已验证。内部入口保留，外部18443仅使用HTTPS设备证书认证，无需账号密码；管理与分发见[外部接入](../docs/public-access.md)。

将仓库源码放入服务器普通目录，由管理员首次安装：

```sh
sudo bash /path/to/ChaoXingHelper/deploy/install.sh
```

可通过 AIGC_PRIVATE_CONFIG 指定已有私有配置文件；没有配置时安装模板，网页正常启动但 ready=false。脚本核对现有入口、容器、网络、/home 挂载和资源；备份入口配置，将两份模板合成为标准 compose.yml，再构建并启动。失败时停止本次服务、恢复入口，保留数据和备份。已有服务目录或 Caddy 基线变化会停止，不能用于覆盖更新。

- 服务目录：/home/docker/services/chaoxing-helper/；数据 data/ 与浏览器配置 browser-state/ 不公开、不入 Git。
- Go 加入 caddy-proxy 和内部 browser-control；Chromium 单独联网，不接入 mihomo-proxy。
- CDP 中继只绑定 browser-control 对应地址，不映射宿主端口。浏览器桌面仅 HTTPS 127.0.0.1:43001，通过管理员 SSH 隧道访问。
- 共享 uploads 只读挂给浏览器；SELinux 使用共享 z 标签，浏览器配置使用共享 z 标签（两个浏览器模式互斥）。
- 面板：https://192.168.1.71/server-readme/；AIGC：https://192.168.1.71/chaoxing/。
- 配置备份位于 backups/，不放在公开文档目录；浏览器镜像按摘要固定。
- 单实例串行检测；内部无登录并可查看检测记录，外部只能读取本人任务及报告。当前保留文档与报告，管理员按实际使用安排清理。

账号登录与只读核验见 [账号接入](../docs/account-connection.md)，真实流程缺项见 [作业检测适配](../docs/platform-workflow.md)。Compose 合成与本地 Go 检查已通过，容器与内部控制通道已验证。

本次通过最新 server-readme 指定的 serverctl 管理通道部署。常规 Docker 构建拉取失败后，使用本机构建的 Linux Go 二进制与逐层校验的官方 Chromium 镜像完成离线安装；对应源码、摘要和备份保存在服务器私有目录。原 codex 账号及 SSH 配置未修改。

按需运行见 [管理说明](../docs/browser-on-demand.md)。管理器源码或 unit 更新后需重新加载并重启 chaoxing-browser-manager.service；默认 Compose 不启动 worker/login profiles。
