erDiagram
    %% ================= 基础字典 =================
    SCHOOL {
        int id PK
        string school_name "学校名称"
        string school_code "学校代码"
        string province "省份名称"
    }

    MAJOR_CLASS {
        int id PK
        string class_name "专业大类名称"
    }

    MAJOR {
        int id PK
        string major_name "专业名称"
        int class_id FK "所属大类ID"
    }

    %% ================= 用户中心 =================
    USER {
        int id PK
        string openid UK "微信OpenID"
        string nickname "昵称"
        string phone "手机号"
        string email "邮箱"
        int school_id FK
        int major_id FK
        int grade "年级"
        int olive_branch_count "付费橄榄枝余额"
        int free_branch_used_today "今日已用免费次数(每日重置)"
        date last_active_date "最后活跃日期(用于重置免费次数)"
        int auth_status "认证状态(0未认证,1已认证,2失败)"
        string auth_img_url "学生证认证图"
        string avatar_url "头像url"
        string cover_image "用户中心封面url"
        boolean email_opt_out "是否退订邮件推广"
        timestamp created_at
    }

    %% ================= 核心业务：人才库 =================
    TALENT_PROFILE {
        int id PK
        int user_id FK "关联用户"
        string self_evaluation "自我评价"
        string skill_summary "技能标签"
        string project_experience "项目经历"
        string mbti "MBTI性格"
        int status "状态(1上架/0下架)"
        boolean is_public_contact "是否公开联系方式"
    }

    %% ================= 核心业务：项目与组队 =================
    PROJECT {
        int id PK
        int creator_id FK "队长(用户ID)"
        string name "项目名称"
        string description "项目详情"
        int school_id FK "所属学校"
        int direction "项目方向(落地/比赛等)"
        int member_count "需求人数"
        int status "审核状态(0待审/1通过/2驳回)"
        int promotion_status "推广状态(0无, 1推广中, 2已结束)"
        timestamp promotion_expire_time "推广结束时间"
        int view_count "浏览量"
        timestamp created_at
        tinyint is_cross_school "是否跨校：1-可以，2-不可以"
        tinyint education_requirement "学历要求1-大专2-本科"
        text skill_requirement "技能要求"
    }

    %% 项目申请记录 (用户 -> 项目)
    PROJECT_APPLICATION {
        int id PK
        int project_id FK
        int user_id FK "申请人"
        string apply_reason "申请理由/留言(已弃用)"
        int status "状态(0待审/1通过/2拒绝)"
        string reply_msg "队长回复(已弃用)"
        timestamp applied_at
    }

    %% 橄榄枝/联系记录 (核心社交：人才->人才, 项目->人才)
    %% 对应“投递橄榄枝”和“可选短信通知”
    OLIVE_BRANCH_RECORD {
        int id PK
        int sender_id FK "发起人ID"
        int receiver_id FK "接收人ID(人才或队长)"
        int related_project_id FK "关联项目ID(若是项目邀请)"
        int type "类型(1:人才互联, 2:项目邀请)"
        int cost_type "消耗类型(1:免费额度, 2:付费额度)"
        boolean has_sms_notify "是否购买短信通知"
        int status "状态(0待处理/1已接受/2已拒绝/3已忽略)"
        timestamp created_at
    }

    %% ================= 运营与增值服务 =================
    %% 商品表 (含：橄榄枝包, 邮件推广服务, VIP等)
    PRODUCT {
        int id PK
        string name "商品名"
        int type "类型(1虚拟币, 2服务权益)"
        decimal price "价格"
    }

    %% 订单管理
    ORDER {
        int id PK
        int user_id FK
        decimal actual_paid "实付金额"
        int status "状态(0未付 1已付 2退款)"
        string wx_pay_no "微信支付单号"
        timestamp pay_time
    }
    
    %% 订单详情
    ORDER_ITEM {
        int id PK
        int order_id FK
        int product_id FK
        decimal price "下单时的单价快照"
        int quantity "数量"
    }

    %% 意见反馈 (管理员：表单反馈管理)
    FEEDBACK {
        int id PK
        int user_id FK
        string content "反馈内容"
        string contact_image "图片凭证"
        int status "处理状态(0待处理/1已处理)"
        string admin_reply "管理员回复"
        timestamp created_at
    }

    %% 消息订阅 (对应MQ逻辑：可选邮件推广/项目订阅)
    SUBSCRIBE_CONFIG {
        int id PK
        int user_id FK
        string target_type "订阅类型(新项目/审核结果/投递进度)"
        string filter_json "过滤条件(如:只看某学校项目)"
        string email "接收邮箱"
        boolean is_active "是否开启"
    }
    EMAIL_PROMOTION {
        int id PK
        int order_id FK
        int project_id FK
        int creator_id FK
        int max_recipients "购买的最大发送人数"
        int total_sent "实际发送数量"
        int status "0-待发送, 1-发送中, 2-已完成, 3-失败"
        string error_message "错误信息"
        timestamp started_at "开始发送时间"
        timestamp completed_at "完成时间"
    }

    %% ================= 关系定义 =================
    
    SCHOOL ||--o{ USER : "属于"
    SCHOOL ||--o{ PROJECT : "归属"
    
    MAJOR ||--o{ USER : "专业"
    MAJOR_CLASS ||--o{ MAJOR : "包含"

    USER ||--o| TALENT_PROFILE : "拥有"
    USER ||--o{ PROJECT : "创建"
    USER ||--o{ ORDER : "支付"
    USER ||--o{ FEEDBACK : "提交"
    USER ||--o{ SUBSCRIBE_CONFIG : "订阅"

    PROJECT ||--o{ PROJECT_APPLICATION : "收到申请"
    USER ||--o{ PROJECT_APPLICATION : "发起申请"

    %% 橄榄枝关系的复杂性
    USER ||--o{ OLIVE_BRANCH_RECORD : "发送橄榄枝"
    USER ||--o{ OLIVE_BRANCH_RECORD : "接收橄榄枝"
    PROJECT ||--o{ OLIVE_BRANCH_RECORD : "作为邀请背景"

    PRODUCT ||--o{ ORDER : "包含"

    USER ||--o{ EMAIL_PROMOTION : "发起"
    ORDER ||--o| EMAIL_PROMOTION : "触发"
    PROJECT ||--o{ EMAIL_PROMOTION : "被推广"