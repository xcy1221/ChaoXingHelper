# AIGC 前后端独立部署

前端源码为 `services/aigc/frontend/`，Vue 3.5.44、TypeScript、Vite，公共主题和排版固定引用 `vendor/` 的 1.0.0 包，依赖锁定于 package-lock.json。业务 CSS 与原页面一致，`aigc`、`aigc-records` 及旧偏好键保留；外部页继续使用原独立样式。

Node 20.19+ 构建：在该目录执行 `npm ci`，设置 `WEB_ASSET_BASE=/_web/aigc/<不可变版本>/` 后执行 `npm run build`。产物为 dist/，index.html 与 records.html 独立入口。API 使用浏览器业务地址的相对路径，兼容内网 /chaoxing/ 与外网 /，不随资源版本改变。

生产链为 Caddy → server-readme Nginx → Go API。前端产物通过 serverctl 静态发布器计划、校验、切换及回滚，保留旧资源；只上传公开 dist/，不挂载数据或服务根目录。每次记录前端源码、公共包与产物指纹及兼容后端。

后端新增 `AIGC_EXTERNAL_WEB=1`，仅关闭旧静态页，API、健康检查、授权、队列及报告不变。先发布兼容后端和 Nginx 基础配置，再切换 Vue。三个页面入口 /chaoxing/、/chaoxing/index.html、/chaoxing/records.html 配置 `auth_request /_aigc-page-access`，子请求经原网关身份访问 GET /api/page-view，并将原 URI 放入 X-Aigc-Page。Go 校验白名单、记录 GET 页面路径并忽略查询参数；不依赖 JavaScript，不能外部读取记录页。

Nginx internal 审计路由不公开，转发保留包含端口的 Host 和原身份头。外网继续由 Caddy 验证设备证书及用户，只放 AIGC 页面、API 和版本下 assets/ 的脚本、样式及图片字体；记录、管理及其他项目保持屏蔽。所有记录和报告 API 继续后端二次校验。

纯前端回退切换上一版本即可。恢复旧合并托管时，先关闭 `AIGC_EXTERNAL_WEB` 并恢复原 Caddy 路由；旧 embed 文件仍随 Go 保留。配置回退不恢复或覆盖数据库。服务器操作仍统一使用 serverctl，并在发布前处理漂移、确认空闲及执行备份。
