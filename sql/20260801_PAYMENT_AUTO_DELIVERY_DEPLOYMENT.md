# 支付自动交付数据库部署顺序

必须先完成数据库迁移及只读验证，再部署依赖新字段和索引的 Go 服务。

按以下顺序执行：

1. `20260801_order_delivery_intent.sql`
2. `20260801_order_delivery_intent_verify.sql`
3. `20260801_email_task_sms_compat.sql`
4. `20260801_email_task_sms_compat_verify.sql`
5. `20260801_message_task_indexes.sql`
6. `20260801_message_task_indexes_verify.sql`
7. `20260802_legacy_pending_delivery_reconcile.sql`

只有三个验证脚本均返回预期字段、索引且不存在无效数据时，才可继续部署后端和消息中心。迁移脚本均按重复执行安全设计；生产执行前仍应保留数据库备份并记录执行回执。
