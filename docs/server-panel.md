# Server Panel 界面规范

内网检测与记录页面遵循 [服务器 README 的 Server Panel 规范](https://192.168.1.71/server-readme/README.md)：

`HOMESERVER · AIGC 文档检测　←返回server panel　外观与排版`

- 单一顶部栏提供顶层返回 `/server-readme/` 和本项目设置；检测记录入口放在内容区。
- 明暗与五种色调使用共享的 `scheduletool-theme`、`scheduletool-tone`；密度及上传／进度区双列或单列布局使用 `aigc-layout-v1`。
- 记录页共用密度，窄屏检测区自动单列；恢复默认仅重置 AIGC 布局。
- 依据服务端 `api/health.canViewRecords` 识别内网，外部页不显示内网导航，不改变设备证书、任务归属或记录鉴权。
- 支持键盘、Escape 关闭与焦点返回；全部脚本和样式同源加载，保持现有 CSP。

发布需保留当前服务器源码中的既有功能，验证检测入口、记录、外部权限与空闲队列后更新嵌入网页的 Go 程序。外观验证不提交真实文档或启动检测任务。

内网业务内容区自适应占满可用空间，移除页面限宽及宽度选项，旧 width 偏好自动忽略；不改变外部证书页的独立展示与权限。
