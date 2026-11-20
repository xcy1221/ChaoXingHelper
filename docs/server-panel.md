# Server Panel 界面规范

内网检测与记录页面遵循 [统一前端维护](https://192.168.1.71/server-readme/frontend.md)，直接复用公共 Vue 顶栏与外观面板。

`HOMESERVER · AIGC 文档检测　←返回server panel　外观与排版`

- 单一顶部栏提供顶层返回 `/server-readme/` 和本项目设置；检测记录入口放在内容区。
- 明暗与五种色调用公共主题，检测与记录的排版分别存于 `serverpanel-page-layout-v1:aigc` 和 `serverpanel-page-layout-v1:aigc-records`，首次读取旧键 `aigc-layout-v1`。窄屏检测区自动单列，恢复默认仅作用于当前页。
- 依据服务端 `api/health.canViewRecords` 识别内网，外部页不显示内网导航，不改变设备证书、任务归属或记录鉴权。
- 支持键盘、Escape 关闭与焦点返回；全部脚本和样式同源加载，保持现有 CSP。

Vue 前端固定引用 `@homeserver/theme`、`@homeserver/ui` 1.1.0。顶栏、外观和排版控件样式由公共组件维护，检测和记录的业务 CSS 留在本项目。发布验证覆盖检测入口、记录、外部权限与空闲队列。外观和上传使用隔离接口与样例。构建与恢复见 [前后端部署](frontend-deployment.md)。

内网业务内容区自适应占满可用空间，移除页面限宽及宽度选项，旧 width 偏好自动忽略；不改变外部证书页的独立展示与权限。

### 页边距与细调

遵循 [Server Panel 规范](https://192.168.1.71/server-readme/README.md)。本页支持左右留白 0–20%、上下留白 0–120 px、密度 60–150%；有卡片网格时另设列数和间距（0–64 px）。保持内容流式宽度、顶部栏与侧栏布局。设置按页面写入 `serverpanel-page-layout-v1:<页面标识>`，首次沿用旧密度和列数，恢复默认只重置当前页面；明暗与色调继续共享。

检测页和记录页分开保存；外部入口保持原布局。

公共组件样式编译进同源 CSS 产物，继续遵循现有 CSP。
