# 合作申请同步到飞书

部署顺序：先在目标数据库执行 `sql/20261027_cooperation_feishu_sync.sql`，再部署后端和后台管理网站。迁移只增加一个商单绑定字段和两张映射表，保留现有商单、接单与申请数据。

后端沿用用户同步的 `FEISHU_APP_ID`、`FEISHU_APP_SECRET` 和 `FEISHU_WIKI_BASE_URL`。另设：

```dotenv
FEISHU_COOPERATION_SYNC_ENABLED=true
FEISHU_COOPERATION_WIKI_SPACE_ID=7701281082951437603
```

平台超管在商单编辑页绑定“快组商业化”知识库中的 Docx 文档。系统在同步时通过飞书 API 核验节点类型和知识库 ID。现已给应用 `cli_aae70d521bf81d02` 授权“明略科技CoCraft产品”文档；其他商单文档需要分别给该应用文档级编辑权限。

审核页的“同步到飞书”会直接在商单文档下创建学校专属知识库多维表格。字段沿用用户导出表的 15 列，并增加申请结果（未处理、成功、失败）、轮次、申请时间、审核时间和审核备注。只同步真实提交过的申请。

此前误建在云空间或使用旧字段的申请表不会被删除；下次同步会新建正确的知识库表格并重建申请映射。旧表格保留供管理员核对。新表格沿用飞书默认权限，您可在表格生成后自行设置成员和访问权限。

同步包含该校合作下的全部申请轮次和当前审核状态。再次点击会更新已有记录，并删除本地已不存在的申请对应的飞书记录。创建表格、记录或文档链接的结果无法确认时，任务进入 `needs_attention`，不会盲目重试创建，以免产生重复数据。此状态需要技术人员核对远端内容和本地映射后处理。

可用以下只读 SQL 验证迁移：

```sql
SHOW COLUMNS FROM commercial_order LIKE 'feishu_wiki_node_token';
SHOW TABLES LIKE 'cooperation_feishu_target';
SHOW TABLES LIKE 'cooperation_feishu_record';
```
