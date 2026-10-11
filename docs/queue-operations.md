# 暂停队列的人工处理

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
