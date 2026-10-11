# 架构（v5）

```text
apps/userscript/src/       油猴源码：视频、作业粘贴
chaoxing-helper.user.js    油猴构建产物
scripts/                  构建
tests/userscript/          油猴回归
services/aigc/             Go 服务、SQLite、检测执行器、嵌入网页
deploy/                   容器配置、面板与部署脚本
```

油猴仍通过单文件安装，功能源码独立维护。作业模块只匹配作答路径，不固化作业参数或授权值。

AIGC 上传 .docx→获得任务标识→查询进度→查看本次摘要及报告。Go 使用标准 HTTP 库提供网页与接口；SQLite 保存队列、报告关联及设备/来源 IP 检测与访问记录。内部无登录；外部由 Caddy 验证 HTTPS 设备证书，Go 限制任务归属，内部记录页不向外部开放。详见[外部接入](public-access.md)。

同一账号、固定作业，全流程串行：重交 Word→教师批阅→触发 AIGC→确认本次结果→保存后处理下一份。重复请求复用原任务；同一请求标识不能换文档。提交、检测标识及文档摘要必须对应当前任务；中断、重启或无法对应时暂停队列，不能盲目重交或覆盖下一份。

Go/chromedp 执行器此前已完成双文档及网页全流程验收。现改为按需无头 Chromium：有任务时启动，空闲 30 秒后关闭；Go 和小型主机管理器常驻。桌面浏览器仅供管理员登录，与无头模式互斥共享账号资料。详见 [按需浏览器](browser-on-demand.md)。本轮原生重登后持久认证恢复；真实冷启动、自动收集报告及空闲退出通过。LAN 限制保持 192.168.1.0/24 与回环。

服务器面板扩展现有 server-readme：Nginx 提供面板与 Markdown，Go 提供 /chaoxing/ 页面，Caddy 仅做反向代理。面板首页移至 /server-readme/，说明首页保留 /server-readme/README.md；现有 OpenClaw 入口保留。面板首版为工具、监控与维护资料入口。

部署目标 /home/docker/services/chaoxing-helper/；持久化 data/ 与 browser-state/。Go 加入 caddy-proxy 和内部 browser-control，CDP 不映射宿主端口；Chromium 桌面仅映射回环地址 43001，由管理员 SSH 隧道访问。部署脚本校验本轮实际读取的入口基线并备份配置。已按最新 server-readme 通过 serverctl 部署，原 codex 账号和 SSH 配置未修改。

Codex 负责设计、实现、审查、集成与部署。DS 只能承担规格明确的基础 coding，不参与设计、Review 或服务器操作。本轮没有调用 DS；旧前端契约因架构变化撤回，避免无必要调用。

账号接入采用服务端专用 Chromium 与 Go/chromedp，首次由管理员完成原生登录。详细设计见 [账号接入方案](account-connection.md)，服务已部署；按需执行的真实全流程已核验。
