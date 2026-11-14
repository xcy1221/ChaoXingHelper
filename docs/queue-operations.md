# 任务队列与历史

## 运行规则

- SQLite 的 jobs 表同时保存队列和历史；任务按入队顺序串行执行，排队及处理中合计最多 20 份，单任务最长 30 分钟。
- 正常状态：queued → uploading → submitting → detecting → waiting → succeeded。同一请求标识与同一文档复用原任务；相同标识不能更换文档。
- 执行异常统一转为 uncertain（待核对），阻止下一任务及新上传；failed 是已结束状态，不阻塞队列。当前提交前登录失败也可能进入 uncertain。
- 重启后 queued 保留，原执行中任务转为 uncertain，避免重复提交。自动重登仅修复会话，不自动解除旧任务造成的暂停。

## 历史与文件

任务保存文档名、创建时间、设备/IP、当前状态、结果及报告关联。原文档位于 data/uploads/，报告位于 data/reports/；完成或标记失败都不会删除文件，目前没有自动清理机制。

内部记录页按时间倒序分页，每页 50 条；外部设备只能读取自己的任务和报告。任务表只保存最新状态，未记录完整阶段时间线。访问记录另表保存最近 10000 条，不包含文档正文或认证秘密。

## 暂停处理

连接中断、报告无法对应或运行中重启，会将任务标为 uncertain，并阻止后续提交。管理员命令仅在本机容器中使用，不提供公开管理接口。

先停止 Go 服务，再查看异常任务：

```sh
docker compose stop chaoxing-helper
docker compose run --rm --no-deps chaoxing-helper --queue-status
```

从私有浏览器核对作业当前附件、检测是否仍在运行以及报告。检测尚未结束就继续等待；需要保留的结果先保存，不能直接恢复下一份提交。

确认平台无进行中的检测，且该异常任务的结果已经保存或决定放弃后，结束该任务：

```sh
docker compose run --rm --no-deps chaoxing-helper \
  --resolve-job 任务的32位标识 \
  --note "核对经过与处理理由" --confirm-platform-checked
docker compose start chaoxing-helper
```

该命令仅将 uncertain 改为 failed，不删除文档、报告或学习通作业，不重交该任务。核对说明只写入私有数据库审计表，普通网页仅显示固定处理消息。仍有 uncertain 或运行中任务时保持暂停或拒绝处理。

需要重新检测时，核对完成后另建新任务；不能复用旧请求标识来触发重复提交。SQLite 恢复测试通过；服务器实测已验证异常暂停和不重复提交的结果收集。

若任务已经完成平台检测，只缺本地结果，可保持网页执行器停用，在私有管理环境运行 /aigc --collect-result 任务ID。它只读取已有检测，核对原文件摘要、当前唯一附件名和新批次，下载合法 ZIP/PDF 后将 uncertain 完成；不提交或重测。任一核对失败则保留暂停状态。首份服务器测试报告已用该方式恢复，第二份完整流程直接完成。

## 维护记录（2026-10-26）

自动重登和冷启动会话复用核验通过后，按用户明确要求将旧 uncertain 任务标为 failed，写入处置审计；原文档保留，未重交。处置后健康接口为 ready=true、paused=false，浏览器恢复空闲待命。此为当时验收结果，当前状态应查询 /chaoxing/api/health。
