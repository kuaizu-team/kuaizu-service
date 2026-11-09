# 快组校园合作：第二阶段交付

日期：2026-10-14。分支：`codex/feishu-user-sync-backend`。

## 阶段边界

数据库迁移、后端接口和通知框架已实现并完成本地测试；微信订阅模板配置 SQL 已按用户确认的 ID 生成，初始 enabled=0；用户已执行并回传核验通过。代理未执行数据库 SQL、连接生产数据库、部署服务或发送真实通知；数据库迁移及模板初始化由用户执行并回传核验。

后台管理网站第三阶段代码已实现，详见其 docs/campus-cooperation-stage3.md。客户端第四阶段代码也已交付：InfoCenter 推荐入口、合作列表/详情、我的申请及消息联动已接入；135 项客户端测试、类型检查与 WXML/WXSS 编译通过。用户选择手动核验模拟器和真机，详见客户端 docs/campus-cooperation-stage4.md。个人中心版本徽章及其他业务保持原有实现，订阅模板仍为 enabled=0。

## 数据与迁移

文件：

- `sql/20261014_01_campus_cooperation.sql`
- `sql/20261014_01_campus_cooperation_verify.sql`
- `sql/20261014_02_campus_cooperation_template.sql`（默认未启用，带键/ID/映射冲突检查）

新增 `commercial_order`、`campus_cooperation`、`campus_cooperation_application`、`campus_cooperation_image`。前者是合作商单，不与支付 `order` 表混用。商单与学校组成唯一合作记录；每次拒绝后重新申请保留新轮次及当时名片快照。

申请表的 VIRTUAL 生成列 `active_user_id` 与合作 ID 的唯一索引，保证同一合作同一用户至多有一条 pending/approved 记录。轮次唯一索引保留所有 rejected 历史。服务事务另外检查最后申请并锁定合作行。

现有 `status_notification` 仅增加可空合作申请引用和对应索引/外键。不回填、删除或重写现有业务、消息、已读标记和优先级。建表脚本不初始化或启用订阅模板；用户已通过独立 02 脚本初始化为禁用。

SQL 使用 IF NOT EXISTS 及 information_schema 条件判断，分别检查列、索引、约束，支持成功后重跑以及 DDL 部分成功后的重跑。MySQL DDL 不能当作整个事务回滚；遇到任何报错应停止并回传报错，不使用忽略错误继续执行。

前置检查确认现有学校关系、消息表和申请来源可空。实际列类型/外键以用户提供的只读核验结果为准；此前数据导出不能代替实际 DDL；现已收到用户执行 information_schema 核验脚本的结果。先运行 verify 脚本可看到现有结构，新表未创建时相关输出为空属于正常；执行迁移后再运行同一 verify 脚本核对新表、生成列、唯一键、索引及队列可空引用。

不新增截止时间、名额、公司 Logo。校区标题最多 20 个 Unicode 字符。TEXT 字段另校验 MySQL 的字节存储上限。联系默认显示，链接和图片默认不显示；新接单默认草稿。

## 权限矩阵

| 角色 | 录入商单 | 接单 | 编辑校区内容/上传图 | 审核申请 |
| --- | --- | --- | --- | --- |
| 平台超管 | 是 | 否 | 所有学校 | 所有学校 |
| 校区当前唯一负责人 | 否 | 仅负责校 | 仅负责校 | 仅负责校 |
| 校区普通管理员 | 否 | 否 | 否 | 仅本校 |
| 赛事管理员 | 否 | 否 | 否 | 否 |

每次请求从数据库检查当前启用账号、角色、学校绑定；写入事务再次锁定授权状态。负责人以 role=2、status=1、`admin_school_relation.is_owner=1` 判定，不把历史接单身份或正数分成关系当作授权依据。负责人交接后由当前唯一负责人继续操作。新业务不修改现有分成、管理员委派及其他业务的学校权限。

## 接口

客户端路径相对 `/api/v2`，复用 JWT 和账号状态检查，学校由当前登录用户确定。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/campus-cooperations` | 分页本校已发布内容；state 为 school-required/no-team/ready |
| GET | `/campus-cooperations/:id` | 本校详情及当前用户最新申请状态 |
| POST | `/campus-cooperations/:id/applications` | 当前名片申请，不需要理由；拒绝后可重申 |
| GET | `/campus-cooperation-applications/my` | 本人各轮申请；不返回内部审核备注及名片快照 |

POST 申请只使用服务端登录用户、当前学校和服务端白名单名片快照，不使用前端传入的 userId、学校、状态或敏感资料。确认弹窗及名片编辑导航在第四阶段复用现有组件。

后台路径相对 `/admin`，复用管理员 JWT。学校以当前授权核验，参数不能扩大权限。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/commercial-orders` | 大厅/平台录入；GET 支持 status 和分页 |
| PUT | `/commercial-orders/:id` | 平台更新名称、公司、说明和可接单状态 |
| POST | `/commercial-orders/:id/claims` | 负责人接单；body `{schoolId}`；同校重复请求返回原记录 |
| GET | `/campus-cooperations` | 所辖学校接单；平台可见所有学校 |
| GET/PUT | `/campus-cooperations/:id` | 读取/完整保存编辑内容 |
| POST | `/campus-cooperations/:id/images` | multipart `file`，JPG/PNG，5MB，返回 `{url,key}` |
| GET | `/campus-cooperations/:id/applications` | 有权审核者查看申请/名片快照 |
| PATCH | `/campus-cooperation-applications/:id` | body `{status: approved/rejected, remark?: string}` |

分页默认 page=1、size=20，size 最大 50，page 最大 1000000。无效分页/ID/能力参数返回 400 并立即终止。跨校资源返回不可访问的 404；角色整体无权限返回 403。

PUT 校区内容字段：title、introduction、links[{title,url,public}]、imagesVisible、imageTitle、images[object_key]、contactsVisible、contacts[{type,label,value}]、applicationRequired、published。linksVisible 仅保留为旧数据兼容字段。标题/介绍必填；每条链接均须填写标题、有效 HTTP/HTTPS 地址及明确的 public 布尔值，链接列表可为空；图片必须属于当前合作。其余模块可空，不限制链接的业务条数。后台读取同时返回 imageUrls 预览地址，保存仍使用 images 中归属已校验的 object_key。

## 申请制与通知

申请制下，public=true 的链接公开，public=false 的链接仅向当前已通过审核者返回，未通过者不获得受限链接的标题和 URL；联系方式仍在通过后按管理员显示开关开放。非申请制下所有链接可见。禁用/下架内容、不同学校、没有启用负责人时不能取得实时详情。

pending/approved 重复申请返回原轮次，rejected 新建下一轮。审核只更新 pending，重复相同决定不重复通知，冲突决定不能覆盖。普通用户拒绝订阅、弹窗失败或没有订阅记录，都不会阻止提交申请。

审核事务一次提交状态、全屏队列、消息归档和通过结果的订阅投递意图。站内通过/拒绝均产生记录；只有通过排入微信订阅投递。事务入队失败会整体回滚；事务提交后的真实发送失败由现有 outbox 机制记录处理，不撤销审核或站内消息。

新类型为 `cooperation-approved` / `cooperation-rejected`。合作通过 priority=200，高于成功类 100、低于移出团队 1000；合作拒绝 priority=10。管理员邀请优先的原入口不变。被压制记录仍保留独立消息归档和未读状态。

新客户端对以下既有接口传 `includeCooperation=true`：

- GET `/users/me/status-notifications/pending`
- GET `/users/me/messages`
- GET `/users/me/messages/unread-count`

不传参数的旧客户端只看原类型和对应未读数；旧待展示查询只压制原类型，不误压制合作通知。邀请优先仍可压制其他全屏队列，归档不变。已读和真实展示确认继续使用原接口，不新增其他消息系统。

消息归档中只保存资源 ID、标题、公司、学校、申请/审核时间、结果，没有用户联系方式或合作联系方式。通知跳转目标是第四阶段计划的 `pages/campus-cooperation-detail/campus-cooperation-detail?id=...`，在新客户端完成前不启用实际通知。

## 订阅模板确认点

代码业务键：`MSG_CAMPUS_COOPERATION_RESULT`。

截图给定 ID：`PiZRtGMWnm2e5oi9KGqjtNE78ORAXJLFf4gJk_TJeEA`。建议 content_json：

```json
{"application":"thing10","result":"phrase3","remark":"thing8"}
```

实际业务内容：application 为校区合作标题（最多20字），result 为“申请通过”，remark 为“点击查看联系方式”。页面路径按具体合作 ID 保存到既有投递记录。框架已完成，用户已确认模板 ID；已交付独立可重复执行的模板配置 SQL，初始 disabled，不伪造平台核验状态。匹配的现有配置保持原启用状态；发生其他业务占用或字段映射冲突时停止，不覆盖。用户已执行该 SQL 并回传完整配置，ID/映射核验通过且 enabled=0；代理未执行 SQL 或写入数据库。

## 契约及验证

正式契约在 `api/common/campus_cooperation.yaml`、两端 `paths/campus_cooperation.yaml`。源码只有一份公共模型；用户接口依既有流程手动注册并在 oapi-codegen 配置排除这些 operationIds，不重复注册。已经按原 bundle/codegen 流程生成类型；生成输出沿用仓库忽略规则，不手工修改。

已通过：

- `go test ./...`（最终代码变动后再次执行相关包测试）。
- Redocly 两端契约打包和 oapi-codegen 类型生成。
- `npx --yes @redocly/cli lint api/service/openapi.yaml api/admin/openapi.yaml --config .redocly.yaml --format json`。
- 本阶段 git diff 空白及变更范围检查。

测试使用 sqlmock、HTTP recorder 和替代派发函数，没有连接生产数据库或真实发送。覆盖角色矩阵、历史分成不授予接单权、当前负责人审核、普通管理员不能编辑、跨校审核/申请/图片、公开链接与私密联系方式、客户端处理器 JSON、授权状态不作为申请准入、重新申请与重复请求、冲突审核、归档/outbox失败回滚、提交后才派发、旧消息范围及原有邀请/优先级回归。

用户执行 SQL 后的结构和模板配置回执已核验通过；本阶段未做实际 MySQL 迁移执行、真实并发数据库验证、微信模板真实投递或客户端设备交互验证。确认模板并核验结构后，按约定阶段推进后台管理网站，再推进客户端页面。

## 2026-10-14 迁移前回执审查

用户回执确认目标库 kuaizu_db，版本 MySQL 8.0.13。现有相关表为 InnoDB/utf8mb4_0900_ai_ci；user.id、school.id 均为有符号 INT，status_notification.id 为有符号 BIGINT，与新增外键对应。现有 application_id 可空，消息归档幂等键/未读索引、负责人生成列及唯一索引存在。四张新业务表及新队列字段尚未出现，符合迁移前状态。

admin_user.school_id 是遗留 BIGINT，不能据此要求修改现有字段；新业务学校引用的是 school.id 的 INT。admin_user.status 允许 NULL，仅凭结构无法断定实际存在 NULL 管理员，补充脚本只统计数量。

联动复核发现原申请表 active_user_id 的 STORED 定义与其基础列 user_id 上的 ON DELETE CASCADE 不兼容。已改为 VIRTUAL，保留复合唯一索引和原定用户删除行为，没有改动数据库。依据：MySQL 官方生成列/外键规则及虚拟列唯一索引支持：
https://dev.mysql.com/doc/refman/8.0/en/create-table-generated-columns.html
https://dev.mysql.com/doc/refman/8.0/en/create-table-secondary-indexes.html

补充只读脚本 sql/20261014_01_campus_cooperation_preflight_extra.sql 核验 InnoDB 页大小/默认行格式、订阅授权历史表/投递索引、实际 NULL 管理员数量、显式重复负责人数量及模板 ID 是否占用。第一份回执未包含这些结果，因此尚未给出最终迁移执行放行。DDL 未在实际 MySQL 中执行验证；Go/sqlmock 测试通过不能替代 DDL 兼容性核验。

### 补充回执审查通过

同日补充回执确认 kuaizu_db / MySQL 8.0.13，innodb_page_size=16384、默认行格式 dynamic；wx_subscribe_delivery 与 wx_subscribe_status_history 为 InnoDB/Dynamic，投递到期扫描、用户/业务时间及授权历史索引均匹配既有迁移。按查询顺序两个计数结果均为 0，即无 NULL 管理员状态、无重复启用负责人。模板查询末尾未见匹配行；模板配置脚本仍自行检查占用，不能仅凭缺少空结果标记跳过冲突检查。

结合第一份回执，已核验的建表迁移前置条件通过，可由用户备份后执行当前修正版 01 迁移，再执行原 verify 脚本并回传结果。此结论只针对本次迁移前置条件，不代表全库业务数据/性能审计或实际 DDL 已成功。模板 02 配置单独初始化为未启用，待客户端目标页面完成并验证后启用。所有脚本仍由用户执行，代理未连接数据库或发送通知。

## 迁移后及最新导出审查

2026-10-14 用户执行修正版 01 后的结构回执确认四表、VIRTUAL active_user_id、唯一索引、新通知可空 BIGINT 引用及关联外键对应均匹配。02 完整回执确认唯一合作业务键、精确模板 ID、application=thing10/result=phrase3/remark=thing8，enabled=0。随后 14:00 最新本地导出复核四表空数据、无悬空学校管理员关系、无重复启用负责人、单条禁用模板及既有通知合作引用空值。可进入后台开发；不追加 SQL、不启用模板。后台第三阶段已实现并通过类型/定向 lint、51 项前端测试及生产构建，浏览器视觉及实际后端/OSS 联调尚未完成。

## 2026-10-26：逐条链接公开权限

沿用 links JSON，每条保存为 {title,url,public}，不新增表、列或路由。

- public=true：无需申请审核通过即可查看。
- public=false：开启 applicationRequired 时，仅当前申请状态为 approved 的用户可查看；未申请、pending、rejected 均不返回该条链接的标题和 URL。
- 未开启 applicationRequired 时，全部链接直接可见。
- 每条链接独立配置，链接顺序不变；联系方式和图片的权限规则保持原样。
- 旧 JSON 缺少 public 时，读取兼容 links_visible（1 为公开，0 为审核后可见）。
- 新管理端保存时必须逐条提交 public；遗漏会提示刷新后台，防止旧编辑页覆盖新的逐条权限。
- 客户端响应 linksVisible 保留，但含义改为“当前用户过滤后的链接列表非空”；小程序旧版可沿用现有模板。

手动执行 SQL：sql/20261026_01_cooperation_link_public.sql。脚本未由 Codex 执行。它只补齐缺少或为 null 的 public，保留已有布尔值、链接顺序与 updated_at；可重复执行。无需变更表结构。

上线顺序：备份 campus_cooperation → 手动执行 SQL 并检查末尾查询 → 同步部署后端与后台 → 刷新已打开的后台编辑窗口 → 发布小程序更新。新后端也能读取未迁移旧数据，但旧后端无法识别逐条权限，因此升级期间暂停编辑链接权限。

本次导出的 CoCraft内测活动（id=2）旧 links_visible=0，迁移后该链接 public=false；当前用户审核通过时后端即返回该链接，无需将其改为公开。SQL 不改申请记录、审核状态、商单、支付或通知。

## 2026-10-26：合作卡片标签与阅读人数

- 列表和详情使用原有接口，新增 tags（有序字符串数组）与 viewCount（去重阅读人数）；图片、联系方式、逐条链接权限及申请审核规则不变。
- tags 复用项目标签规则（0–5 个，每个 1–12 字）。当前有编辑权限的管理员保存合作时，在同一事务中将标签写入 project_tag 并保存到合作 tags JSON；已有标签复用，未存在的名称自动入库。旧编辑端遗漏 tags 时保留原值，显式 [] 清空。
- 唯一新增接口为后台 GET /project-tags?keyword=...&limit=12，复用现有管理端认证与合作权限，提供有上限的前缀优先联想；% 和 _ 按字面量搜索。没有新增页面或客户端导航路由。
- 阅读人数来自 campus_cooperation_view 的唯一键 (cooperation_id,user_id)。只有通过现有登录、学校、发布状态及运营团队检查的详情请求才记入；重复访问、刷新和管理员预览不重复计入。统计失败不打断已授权的详情显示。
- 手动执行 sql/20261026_02_cooperation_cards.sql 后再部署后端，然后部署后台、发布小程序。脚本未由 Codex 执行；新增 tags JSON 列和去重阅读表，不修改已有链接、申请、支付或通知数据。历史没有采集过的阅读人数从 0 开始。
