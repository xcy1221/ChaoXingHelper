# ChaoXingHelper

学习通工具。油猴 v0.4.1 提供视频辅助与作业文本粘贴。AIGC、专用浏览器和服务器面板已部署，Word 提交到报告下载已实测。[内网检测](https://192.168.1.71/chaoxing/)与[内部记录](https://192.168.1.71/chaoxing/records.html)无需登录；[外部入口](https://221.216.94.213:18443/)仅需设备证书，无需账号密码，接入与撤销见[说明](docs/public-access.md)。

## 油猴脚本

在 Tampermonkey 中安装或覆盖根目录的 chaoxing-helper.user.js。源码按模块维护，单文件是浏览器安装所需的构建产物。

- 视频：手动播放一次后点击“开始连播”；点“停止”暂停。等待平台确认任务点后切换，90 秒未确认则停止。
- 作业：在 mooc1 或 mooc1-api 的 /mooc-ans/mooc2/work/dowork 页面直接粘贴文本，支持选区、换行与同源编辑器 iframe；不会自动提交。
- 两个模块按页面分别启动。禁用脚本并刷新即可恢复原行为。

```sh
npm run build:userscript
npm test
```

## AIGC 网页

支持 .docx 文件（最大 10 MB）、任务进度和报告下载。Go 同源提供网页与接口；SQLite 保存队列、报告关联及检测记录。内部无需登录，外部认证由 Caddy 处理，并限制任务和报告归属。检测未接入时显示“未就绪”并禁止上传。

需要 Go 1.26 或更新版本：

```sh
cd services/aigc
go test ./...
go run .
```

本地地址为 http://127.0.0.1:8080/。可用 AIGC_LISTEN、AIGC_DATA_DIR 配置监听与数据目录。未配置或未启用平台执行器时 ready=false。使用 Go/chromedp 控制已登录的专用浏览器；配置与实测状态见 [作业检测适配](docs/platform-workflow.md)。本地模拟测试不代表完整平台验收。

## 仓库与部署

- apps/userscript/：油猴模块；scripts/ 和 tests/userscript/：构建与回归。
- services/aigc/：Go 服务、嵌入网页和后端测试。
- deploy/server-panel/：现有 server-readme 的面板扩展。
- [架构](docs/architecture.md) · [部署说明](deploy/README.md)

部署延续服务器既有 Nginx、Caddy 与 Docker 网络。Go 提供 AIGC 页面，Nginx 提供面板，Caddy 仅反向代理。

## 验证与数据

油猴模拟回归、Go 后端测试和 Linux 编译通过。登录浏览器中的测试 Word 上传、重交、AIGC 检测及报告对应已实测。服务器 Go/CDP 已完成两份不同 Word 的串行重交、检测与报告保存；异常暂停及只读结果收集通过。真实编辑器的脚本粘贴、模型同步和提交保存通过，原答案已恢复；此项使用模拟剪贴板事件，未冒称原生 Ctrl+V 实测。

账号、Cookie、密钥、上传文档与报告不入 Git。外部只能读取本人任务；检测记录仅内部可见。服务器限制非 LAN 的 IPv4/IPv6 入站及 Docker 转发，只开放 AIGC18443。

仓库：https://github.com/xcy1221/ChaoXingHelper

浏览器资源策略见 [按需运行](docs/browser-on-demand.md)：移除分钟级页面轮询，任务时启动无头浏览器，空闲后停止；桌面仅用于管理员登录。
