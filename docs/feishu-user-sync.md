# 用户名单同步到飞书

CSV 导出已替换为单学校飞书同步，原 POST /admin/users/export 不再注册。批量审核、用户查询及其他导出功能保持原实现。网站代理也关闭旧 CSV 入口。

## 配置与发布

后端用自建应用凭证获取 tenant_access_token，直接调用官方 OpenAPI，不需要服务器安装 CLI 或用户 OAuth。官方 @larksuite/cli 用于授权配置和接口联调。

在 admin 服务环境变量或受控 .env 中配置：

```dotenv
FEISHU_USER_SYNC_ENABLED=false
FEISHU_APP_ID=cli_aae70d521bf81d02
FEISHU_APP_SECRET=<在服务端私密配置中填写>
FEISHU_WIKI_SPACE_ID=7694296425655438517
FEISHU_WIKI_PARENT_NODE_TOKEN=Ohzzwy7AIiMQfukniozc6dvNnGh
FEISHU_WIKI_BASE_URL=https://rw0q02v5pok.feishu.cn
```

.env.example 默认关闭同步，Secret 留空。不能把 CLI 的凭证存储引用当作 Secret；当前 CLI 授权不等于已配置服务器 Secret。Secret 不进入前端、接口响应或版本控制。

应用身份权限需要 wiki:wiki、bitable:app；“快组儿们”的首页需添加该应用并授予管理权限。这套授权已通过应用身份联调，不需要另一套 Base v3 权限。

启用前手动执行 sql/20261008_feishu_user_sync.sql，创建三张新表，不修改用户数据。配置异常时禁用同步，其他后台功能继续运行。MySQL 连接池上限至少为 2，默认 50；多个 admin 实例需连接同一 MySQL 写入节点，连接链路需支持会话级 GET_LOCK。

上线顺序：迁移、配置服务端 Secret、保持同步关闭并先发布 admin 再发布网站、核对版本与配置后开启同步并重建 admin 容器、选择已确认学校同步两次核验行数与权限。更新 GitHub Actions ENV_DOCKER 后由部署流程写入服务器 .env.docker；仅修改环境文件不会改变已运行容器中的环境变量。首次上线按此顺序执行；用户已完成初版真实学校同步，后续批量更新优化只需发布新的 admin 版本。

## 范围与接口

| 角色 | 规则 |
| --- | --- |
| 1 平台超管 | 显式指定一个有效学校；没有全部学校同步 |
| 2 校区超管 | 显式选择一个数据库 commission_rate > 0 的授权学校 |
| 3 普通校区管理员 | 最新数据库绑定学校；未绑定或指定其他学校拒绝 |
| 4 赛事管理员及未知角色 | 禁止 |

请求及后台任务均读取最新管理员状态。禁用、角色变化、授权撤销会停止后续写入。同步本校全量，不使用列表筛选或勾选。无学校用户排除。

- POST /admin/users/feishu-sync：接受单个 schoolId；角色 3 可省略，使用绑定学校。拒绝额外的旧导出参数。同校已有 queued/running 任务时返回现有任务。
- GET /admin/users/feishu-sync?schoolId=...：查询授权学校最新任务，无任务返回 data:null。
- 返回 id、schoolId、status、total、processed、created、updated、deleted、message、createdAt、updatedAt 和可用时的 url；不包含操作者 ID 或用户字段。
- 状态为 queued、running、succeeded、failed、needs_attention。

## 15 列合同

每个学校首次同步会在首页下创建独立 Bitable，标题含学校名称和 ID。业务数据表及默认视图名为“用户名单”。飞书可能附一个初始化的空数据表；同步仅使用映射中的业务表，不接管其他表。

| 顺序 | 原列名 | 类型 | 含义 |
| --- | --- | --- | --- |
| 1 | 昵称 | 文本、主字段 | user.nickname |
| 2 | MBTI | 单选 | 名片 mbti，16 种标准值 |
| 3 | 学校 | 文本 | 关联学校名称 |
| 4 | 专业 | 文本 | 关联专业名称 |
| 5 | 入学年份 | 文本 | 原 grade，保留历史年级值 |
| 6 | 自我介绍 | 文本 | 名片 self_evaluation |
| 7 | 项目经历 | 文本 | 名片 project_experience |
| 8 | 协作等级 | 单选 | 原 CollaborationLevel：极好/优秀/良好/中等/较差 |
| 9 | 协作具体分数值 | 数字，0.00 | 持久化分数，按原 CSV 两位小数规则取值 |
| 10 | 是否通过学生认证 | 单选 | auth_status=1 为是，其余为否 |
| 11 | 是否入驻人才库 | 单选 | 名片 status=1 为已入驻人才库，其余为未入驻人才库 |
| 12 | 电话 | 文本 | 原手机号，保留前导零 |
| 13 | 微信号 | 文本 | 原微信号 |
| 14 | 邮箱号 | 文本 | 原邮箱 |
| 15 | 账号状态 | 单选 | 0/其他：正常，1：封禁，2：已毕业 |

沿用原导出 LEFT JOIN 与字段表达式，学校条件复用 UserFilterSQL。缺失名片、专业、分数仍保留用户行。空值用 null 清除旧单元格；原始文本和换行保留，不添加 CSV 保护单引号。没有额外的用户 ID、学籍或任务状态列。

写入前检查默认视图下的 15 列名称、顺序、类型、标签及分数格式。被手工变更时停止，不自动转换类型而丢失内容。出现未支持 MBTI 时明确失败，不静默替换。

## 映射、任务与清理

feishu_user_sync_target 保存学校→节点/Bitable/数据表/视图及创建阶段。feishu_user_sync_record 以 (school_id,user_id) 为业务键，保存 record_id、UUID client_token、新增状态及临时 payload。feishu_user_sync_job 保存任务和统计，学校行事务锁加唯一 active_school_id 防止重复入队。

后台 worker 持有独立 MySQL 会话锁，跨实例串行执行，每个实例最多每秒 2 个飞书请求。写入前检查锁和最新权限；释放锁失败时丢弃物理连接，避免持锁连接回池。页面断开不取消任务；单个飞书请求最多 20 秒，任务没有整校人数上限。

有映射则更新全部 15 列，无映射则新增。以数据库内容覆盖受管记录，不用本地哈希跳过更新。已有记录按最多 50 人分页读取本地映射，字段仍逐人按学校范围查询；通过原生 records/batch_update 接口串行更新，每批最多 50 条且 JSON 请求体不超过 1 MiB。超过字节预算时提前分批；单条超过预算的记录走原有单条更新流程。权限及锁状态在每页开始和每批写入前复核，写入前用带学校条件的 IN 查询再次排除已转校或删除的用户。新增用户继续逐条保存 pending/client_token、创建、确认映射，不使用批量新增。

每批响应必须确认全部请求记录 ID，顺序可以不同，但缺失、重复或意外 ID 都会停止任务并保留映射；网络失败及不完整响应不会触发补插。明确 RecordIdNotFound 时将该批拆成两半定位缺失记录，不假设批量接口失败意味着全部未写入；定位到单条后仍由原单条接口确认缺失，才允许清理该受管映射并进入原防重复新增流程。表格不存在、无权限等错误不能当作用户行丢失。

更新进度在每个确认成功的批次保存，新建进度仍逐条保存；页面可能从 0/38 直接跳到 38/38。任务日志记录学校 ID、任务 ID、耗时和统计；批次日志仅记录条数、请求耗时和确认状态。日志不记录用户字段、应用凭证或完整飞书资源 URL。这次优化不需要数据库迁移、新环境变量、追加应用 scope、前端页面或接口调整。

仅清理本校映射中已无本校用户的行，删除前重查归属。人工未映射行完全不扫描或删除。转校后新学校建立自己的映射，旧学校记录在旧学校下次同步时清理。删除失败保留映射。任务开始和结束各清理一次。

数据库与飞书没有跨系统事务。同步以各次查询快照为准，并发转校和远端写入不能原子提交，结束清理及下一次同步负责收敛。

## 中断与核对

节点、表格、记录新建前先持久化创建阶段；新增记录先保存 client_token 和原始 payload，成功后保存 record_id 并清空 payload。

丢失响应、进程中断、成功后映射保存失败，可能留下未确认新增。状态变为 needs_attention，系统不自动重新创建。已验证相同 client_token 重放得到同一记录，但官方文档没有明确其有效期限，因此不依赖无限期重放保证不重复。

先暂停该学校任务，再核对远端与本地映射：

1. 节点或表已存在：确认由本次应用请求创建，补全学校 node_token、app_token、table_id、view_id。不要盲目清空创建阶段。
2. 记录已存在：确认唯一对应用户，补全正确 record_id，设 state=ready 并清空 payload。不能仅用同名昵称猜用户身份。
3. 只有明确证明远端未创建，才能重置对应创建阶段或移除 pending 映射。不能以“查不到”代替“确认未创建”。
4. 核对后重新同步。结构漂移先恢复原列与标签。

不再持锁的中断 running 任务标记失败，保留已完成进度与映射，不自动重放未知新增。数据库异常时状态可能暂时保持 running，恢复后 worker 更新状态。

## 当前验证

初版已通过同步隔离测试、管理员/repository/service 回归、go vet，以及网站组件、类型和 ESLint 检查。真实飞书虚构数据联调验证了首页下新建 Bitable、15 列建表、幂等新增、更新、空字段、原文回读及删除，两条虚构行均已清理。用户已完成三张同步表迁移、服务器及 Actions 配置、前后端部署，并确认真实学校同步成功（38 人约 2 分钟）。

2026-10-09 在 codex/feishu-user-sync-batch-update 分支优化已有记录的批量更新。新增隔离回归覆盖 38 人一次更新、51 人分页、更新与逐条新增混合、UTF-8/JSON 转义后的请求大小、超大单条回退、转校与撤权过滤、未知新增不重插、完整响应确认、明确缺失的定位重建以及异常时保留已确认进度。尚未推送或部署这次优化；实际提速需部署后测量，首次全部新增仍采用原逐条新增机制。

官方参考：[官方 CLI](https://github.com/larksuite/cli)、[知识库建节点](https://open.feishu.cn/document/ukTMukTMukTM/uUDN04SN0QjL1QDN/wiki-v2/space-node/create)、[建数据表](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table/create)、[新增记录](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table-record/create)、[更新记录](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table-record/update)、[批量更新记录](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table-record/batch_update)。
