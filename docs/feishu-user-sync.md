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

上线顺序：迁移、配置服务端 Secret、保持同步关闭并先发布 admin 再发布网站、核对版本与配置后开启同步并重建 admin 容器、选择已确认学校同步两次核验行数与权限。更新 GitHub Actions ENV_DOCKER 后由部署流程写入服务器 .env.docker；仅修改环境文件不会改变已运行容器中的环境变量。完整“管理员页面→目标数据库→飞书”的真实学校联调仍待这一步完成。

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

有映射则更新全部 15 列，无映射则新增。以数据库内容覆盖受管记录，不用本地哈希跳过更新。仅在明确 RecordIdNotFound 时清理该映射并允许重建；表格不存在、无权限等错误不能当作用户行丢失。

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

已通过同步隔离测试、管理员/repository/service 回归、go vet，以及网站 38 项测试、TypeScript 和改动文件 ESLint。测试涵盖 15 列、原文、空值、枚举、学校范围、权限撤销、新增更新映射、未知新增不重插、仅清理映射、删除失败保留映射、重复点击和中断恢复。

真实飞书联调仅用虚构数据，验证首页下新建 Bitable、15 列建表、幂等新增、更新、空字段、原文回读及删除。两条虚构行均已清理，验证 Bitable 保留供人工检查。2026-10-09 用户已手动完成 kuaizu_db 三张同步表迁移及只读验收：表、字段及索引符合脚本，三表行数均为 0。用户已保存服务器配置及后端 GitHub Actions ENV_DOCKER；当前同步保持关闭，未同步真实用户，未部署或提交本次代码。竞态检测因本机 CGO 未启用且无 C 编译器未完成。曾完成一轮默认生产构建；最终小幅页面修正后的构建复验被原有 Google 字体下载网络故障阻断，Webpack 复验也受同一网络限制。最终版本类型检查及组件测试已通过，上线前需在能访问 Google Fonts 的环境重做生产构建。

官方参考：[官方 CLI](https://github.com/larksuite/cli)、[知识库建节点](https://open.feishu.cn/document/ukTMukTMukTM/uUDN04SN0QjL1QDN/wiki-v2/space-node/create)、[建数据表](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table/create)、[新增记录](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table-record/create)、[更新记录](https://open.feishu.cn/document/server-docs/docs/bitable-v1/app-table-record/update)。
