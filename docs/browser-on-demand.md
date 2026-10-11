# 按需浏览器

Go 网页常驻；无头 Chromium 和 CDP 中继由主机 systemd 管理器按任务唤醒，队列空闲 30 秒后停止。空队列不创建互斥文件，也不打开学习通页面。桌面浏览器仅在管理员登录模式运行。

网页后端没有 Docker socket。主机 browser-manager.py 只操作三个固定浏览器容器，读取队列和状态；它与 Go worker 使用同一排他锁，防止空闲关闭撞上提交。uncertain 仍阻塞下一任务，登录模式不接受新任务。

ready=true 在此模式表示可以排队；实际账号、学生和附件在任务开始时核验。连接或身份核验失败会暂停队列，不盲目重交。

## 管理员登录

队列须为空，执行：
```sh
sudo python3 /home/docker/services/chaoxing-helper/browser-manager.py --login
```

用原管理员 SSH 隧道打开 https://127.0.0.1:43001，访问学习通并勾选“下次自动登录”。无需传递密码或 Cookie。完成后执行 --resume。

只读检查时先执行 --wake，再运行 docker exec chaoxing-helper /aigc --inspect-platform，完成后 --sleep。手动唤醒最多两分钟，之后按空闲策略退出。修复会话后仍须人工核对 uncertain；重启 Go 不能自动重交。

## 部署与资料

主机服务：chaoxing-browser-manager.service。worker 和 login Compose profiles 不随默认 compose up 启动；管理员使用上述入口。profile 与数据仍在原持久化目录，两个浏览器模式互斥共享。Chrome 原有 no-sandbox 包装参数保留；容器权限未扩大。

2026-10-11 已清理离线导入的 121 个分块、镜像层和归档，释放 8,615,800,832 字节。Docker 镜像及校验 JSON 保留，账号和报告未删除。清理清单保存在服务器私有 backups/install-cache-cleanup-20261011.json。

2026-10-11 已完成重登、真实任务冷启动、自动报告对应核验和结束退出；登录资料跨多次浏览器停止保持有效。LAN 提交已恢复。Go 维护重启即使停用执行器，也会清除自身遗留的 worker 锁，防止阻塞空闲关闭。

75 秒正常待命采样：Go 与主机管理器合计平均 CPU 约单核的 0.193%，内存工作集合计 32.1 MiB；三浏览器容器均停止。此为实测样本，不代表零资源消耗。

浏览器字体保存在 browser-state/.local/share/fonts（Noto Sans 常规与斜体，共 1,302,932 字节），替代规则见 [browser-fonts.conf](../deploy/chaoxing-helper/browser-fonts.conf)。规则安装于 browser-state/.config/fontconfig/conf.d/99-chaoxing-latin.conf，两种浏览器共用；目录属主 65532，字体及规则可读。新建 profile 时需从主机已有 /usr/share/fonts/google-noto-vf/ 复制这两份字体并安装规则，再冷启动浏览器。它修复 Arial/Helvetica/Segoe UI 缺失时日期数字落到 Emoji 的问题，不下载或重建镜像。
