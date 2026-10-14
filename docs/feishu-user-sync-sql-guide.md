# 手动数据库调试顺序

当前决定：先完成本地隔离联调，正式同步保持关闭。数据库脚本由您手动执行；助手不连接或修改正式数据库。

优先在测试库执行。若先在正式库验收迁移，仍保持 FEISHU_USER_SYNC_ENABLED=false；迁移只创建三张新表，不改现有用户、学校、管理员、名片数据，也不会发起同步。

在数据库客户端选择正确的数据库，然后依次执行：

1. sql/20261009_feishu_user_sync_preflight.sql：只读预检。确认 selected_database 正确，missing_column 和 duplicate_profiles 结果为空。索引结果需有学校查询索引，以及人才名片用户唯一索引。
2. sql/20261008_feishu_user_sync.sql：创建目标映射、记录映射、任务状态三张表。没有 DROP、DELETE、TRUNCATE，也不修改用户表。
3. sql/20261009_feishu_user_sync_verify.sql：只读验收。应看到三张 InnoDB 表；target 主键 school_id，record 主键 school_id+user_id、唯一 client_token，job 主键 id、唯一可空 active_school_id。新安装三表行数应为 0。

已有同步数据时保留，不要为了得到 0 行而清表。若三表已存在，IF NOT EXISTS 不会修复不同结构，请把验收结果返回后再处理。

执行后反馈是否成功；有错误时提供错误信息，以及 selected_database、缺失列结果、三张表和索引验收结果。无需提供数据库密码、用户联系方式或名片内容。

本地环境暂用隔离测试和虚构飞书记录验证。完整页面联调需要测试数据库可用、admin 服务配置真实 App Secret 后才能启动。App Secret 只写入服务端私密环境文件。当前不会读取真实后端 .env 来猜数据库环境，也不会启动包含消息恢复任务的完整 admin 服务连接正式库。

预检会列出少量学校候选及人数。真实首轮联调学校将在环境确认后，从授权范围内选择人数较少的学校；不会直接同步全平台。迁移本身不会触发同步；后续由后台按钮和学校权限校验执行。
