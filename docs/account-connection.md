# 学习通账号接入

Go/chromedp 执行器、持久化 Chromium 和私有 CDP 中继已在服务器运行，首次登录及只读核验通过。Go 提供网页、串行队列与结果，普通使用者无需登录。

## 管理员接入

1. 管理员运行部署脚本，在私有 data/account/platform.json 填入有效作答及批阅地址、目标学号；enableAutomation 保持 false。
2. 可按下文配置私有自动登录凭据；需要人工验证时，先执行主机 browser-manager.py --login（无排队或执行中任务），再通过已有 SSH 建立隧道：`ssh -N -L 43001:127.0.0.1:43001 xcy@192.168.1.71`，打开 https://127.0.0.1:43001。不修改 SSH 配置。
3. 使用桌面时完成学习通原生登录与验证，勾选“下次自动登录”，然后执行 --resume。同一账号需要能访问学生作答页和教师批阅页；platform.json 不存密码，自动登录凭据单独保存在 login.json。
4. 先执行 browser-manager.py --wake，再从服务目录执行 `docker compose exec chaoxing-helper /aigc --inspect-platform`。它只核对目标作业、选中学生、双端附件和报告入口，不提交或触发检测。
5. 完成两份不同 Word 的真实上传、重交与报告关联验收后，再将 enableAutomation 改为 true，重启 Go 服务。现有部署已完成该验收并启用；内部和外部设备的访问边界见[外部接入](public-access.md)。

浏览器桌面使用镜像自带 HTTPS。自签名证书的信任由管理员在本机处理；该入口不经 Caddy 发布。人工登录维护前先停 Go 服务，避免浏览器并发操作；任务开始时的自动重登由执行器完成。重新启动不会自动重交副作用不确定的任务。

## 会话与控制范围

browser-state/ 保存独立 Chromium profile；data/account/ 保存私有地址、自动登录凭据与提交前快照，均不进入 Git、日志或公开 server-readme 目录。上传目录只读共享给浏览器；浏览器没有 Docker socket、privileged 或其他宿主目录。

CDP 原生端口 9222 留在浏览器容器回环地址，中继仅绑定 Docker 内部 browser-control 网络的本机地址，Go 通过 chromium-control:9223 访问。中继仅代理浏览器元数据与 WebSocket，拒绝带 Origin 的网页请求；不映射宿主端口。浏览器桌面端口仅映射宿主 127.0.0.1。

Chromium 镜像按摘要固定，PUID/PGID 为 65532；profile 与上传使用 SELinux 共享 z 标签，桌面与无头模式互斥。实际身份在任务开始时核验，不能仅凭 profile 存在认为已登录。

## 自动重登

2026-10-26 已部署并实测自动重登；浏览器停止后冷启动，在不提供凭据文件的核验进程中复用会话也通过。使用私有 `data/account/login.json`，内容为 `{"phone":"手机号","password":"密码"}`，属主须与服务一致，权限 `0600`。仅在检测前发现官方登录页时读取，走网页手机号密码登录，每次任务最多尝试一次；验证码、二次认证或登录失败即停止。已有会话继续复用。凭据不进入源码、公开页面或日志。自动重登不会重交 uncertain 任务，也不自动更新过期作答签名。

服务器上的文件位置为 `/home/docker/services/chaoxing-helper/data/account/login.json`，现有服务 UID/GID 为 `65532:65532`。不要将账号密码放入命令参数或聊天记录；更新文件后，无需重启服务，下一次需要重登时才读取。`--inspect-platform` 遇到登录页也会尝试重登，但不提交作业或触发检测。

## 当前边界

只读核验验证的是页面上所选学生和固定作业绑定，尚未独立识别当前登录的教师 UID。有效配置下可检测连接和页面访问失败；详细会话状态机和登录身份变化核验仍待实现。异常队列的本机处理见 [人工核对](queue-operations.md)，不会自动重交。

作答临时签名目前读取私有配置，过期会停用；从课程导航自动更新尚未实现。两份 Word 串行对应已实测；隔离副本重启后暂停、独立无登录窗口访问被拦截也已验证。网页不提供 Cookie 或会话控制接口，CDP 不经反向代理公开。

参考：[chromedp](https://github.com/chromedp/chromedp)、[Chromium 镜像](https://docs.linuxserver.io/images/docker-chromium/)、[Chrome 独立调试目录要求](https://developer.chrome.com/blog/remote-debugging-port)。

管理命令实测时保持 enableAutomation=false：--verify-platform 后接一至两份生成的测试 Word；--verify-queued 后接明确的 queued 任务 ID。两者不监听端口、不写入持久启用开关；有未处理队列或 uncertain 时拒绝开始新实测。它们仍会实际提交、检测，不是只读命令。

本机页面与 SSH 隧道关闭后，服务端任务继续；空闲 30 秒关闭浏览器，登录资料保存在独立 profile。Debian 书签来自 Chromium 容器的默认配置，宿主仍为 CentOS。内部入口与外部设备证书入口并存，浏览器桌面和 CDP 仍不对外公开，见[外部接入](public-access.md)。

当前采用 [按需浏览器](browser-on-demand.md)。2026-10-11 重登后认证 Cookie 已持久化，桌面切换无头及多次停止唤醒均核验有效；未读取 Cookie 值。此前 profile 存在但认证未恢复，不能仅凭 Cookie 文件判断登录有效。学校新版首页使用 https://i.chaoxing.com/base?ws=1；2026-10-11 按本机截图尺寸核对，蓝色学校主题与八列课程布局一致。原远程入口属于旧个人空间。另已修复容器数字使用 Emoji 字体的问题，见 [按需浏览器](browser-on-demand.md)。
