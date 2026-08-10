-- MySQL dump 10.13  Distrib 8.0.19, for Win64 (x86_64)
--
-- Host: kuaizu-db.rwlb.rds.aliyuncs.com    Database: lianxi
-- ------------------------------------------------------
-- Server version	8.0.13

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;
SET @MYSQLDUMP_TEMP_LOG_BIN = @@SESSION.SQL_LOG_BIN;
SET @@SESSION.SQL_LOG_BIN= 0;

--
-- GTID state at the beginning of the backup 
--

SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '09c0ed11-3a14-11f0-9fc2-00163e0c6f4a:1-16302';

--
-- Table structure for table `admin_user`
--

DROP TABLE IF EXISTS `admin_user`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `admin_user` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `username` varchar(50) NOT NULL COMMENT '用户名',
  `password_hash` varchar(255) NOT NULL COMMENT 'bcrypt密码哈希',
  `nickname` varchar(50) DEFAULT NULL COMMENT '显示名称',
  `role` tinyint(4) NOT NULL DEFAULT '3' COMMENT 'admin role:1-super,2-school super,3-school admin',
  `school_id` int(11) DEFAULT NULL COMMENT 'bound school id',
  `finance_remark` varchar(500) DEFAULT NULL COMMENT 'finance remark',
  `commission_rate` decimal(5,2) NOT NULL DEFAULT '0.00' COMMENT 'commission rate percent',
  `status` tinyint(4) DEFAULT '1' COMMENT '状态:1-启用,0-禁用',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='管理员用户表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `email_promotion`
--

DROP TABLE IF EXISTS `email_promotion`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `email_promotion` (
  `id` int(11) NOT NULL AUTO_INCREMENT,
  `channel` varchar(32) DEFAULT NULL COMMENT '消息渠道',
  `business_tag` varchar(64) DEFAULT NULL COMMENT '业务标签',
  `trace_id` varchar(128) DEFAULT NULL COMMENT '业务追踪ID',
  `order_id` int(11) NOT NULL COMMENT '关联订单',
  `project_id` int(11) DEFAULT NULL COMMENT '推广的项目',
  `creator_id` int(11) NOT NULL COMMENT '发起人（队长）',
  `strategy` varchar(32) NOT NULL DEFAULT 'region' COMMENT '收件人选择策略',
  `max_recipients` int(11) NOT NULL COMMENT '购买的最大发送人数',
  `total_sent` int(11) DEFAULT '0' COMMENT '实际发送数量',
  `status` tinyint(4) DEFAULT '0' COMMENT '0-待发送, 1-发送中, 2-已完成, 3-失败',
  `error_message` text COMMENT '错误信息',
  `started_at` timestamp NULL DEFAULT NULL COMMENT '开始发送时间',
  `completed_at` timestamp NULL DEFAULT NULL COMMENT '完成时间',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `fk_email_promotion_order` (`order_id`),
  KEY `idx_project` (`project_id`),
  KEY `idx_status` (`status`),
  KEY `idx_ep_business_trace` (`channel`,`business_tag`,`trace_id`),
  KEY `idx_ep_project_order` (`project_id`,`order_id`),
  KEY `idx_ep_reconcile` (`channel`,`business_tag`,`status`,`id`),
  CONSTRAINT `email_promotion_order_fk` FOREIGN KEY (`order_id`) REFERENCES `order` (`id`) ON DELETE CASCADE ON UPDATE RESTRICT,
  CONSTRAINT `fk_email_promotion_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邮件推广记录表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `email_provider_config`
--

DROP TABLE IF EXISTS `email_provider_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `email_provider_config` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `provider_type` varchar(50) NOT NULL COMMENT '服务商类型：aliyun/smtp/sendgrid',
  `config_name` varchar(100) NOT NULL COMMENT '配置名称',
  `config_json` json NOT NULL COMMENT '配置参数JSON',
  `is_default` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否默认：0-否 1-是',
  `is_active` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用：0-禁用 1-启用',
  `priority` int(11) NOT NULL DEFAULT '0' COMMENT '优先级（数字越小优先级越高）',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_provider_type` (`provider_type`),
  KEY `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邮件服务商配置表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `email_task`
--

DROP TABLE IF EXISTS `email_task`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `email_task` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `task_key` varchar(255) NOT NULL COMMENT '幂等任务键',
  `channel` varchar(32) NOT NULL COMMENT '消息渠道',
  `business_tag` varchar(64) DEFAULT NULL COMMENT '业务标签',
  `trace_id` varchar(128) DEFAULT NULL COMMENT '业务追踪ID',
  `promotion_id` int(11) DEFAULT NULL COMMENT '邮件推广记录ID；短信任务为空',
  `recipient_email` varchar(255) DEFAULT NULL COMMENT '收件人邮箱',
  `recipient_phone` varchar(32) DEFAULT NULL COMMENT '收件人手机号',
  `template_code` varchar(50) NOT NULL COMMENT '使用的模板编码',
  `provider_id` int(11) DEFAULT NULL COMMENT '服务商配置ID',
  `template_vars` json DEFAULT NULL COMMENT '模板变量JSON',
  `status` tinyint(1) NOT NULL DEFAULT '0' COMMENT '状态：0-待发送 1-发送中 2-成功 3-失败 4-重试中',
  `retry_count` int(11) NOT NULL DEFAULT '0' COMMENT '重试次数',
  `external_id` varchar(255) DEFAULT NULL COMMENT '供应商消息ID',
  `error_code` varchar(64) DEFAULT NULL COMMENT '供应商错误码',
  `error_msg` varchar(500) DEFAULT NULL COMMENT '错误信息',
  `send_time` timestamp NULL DEFAULT NULL COMMENT '实际发送时间',
  `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_email_task_task_key` (`task_key`),
  KEY `idx_email_task_promotion_channel_status` (`promotion_id`,`channel`,`status`),
  KEY `idx_email_task_recipient_promotion` (`recipient_email`,`promotion_id`),
  KEY `idx_status` (`status`),
  KEY `idx_create_time` (`create_time`),
  KEY `idx_email_task_identity_latest` (`channel`,`business_tag`,`trace_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邮件发送任务表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `email_template`
--

DROP TABLE IF EXISTS `email_template`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `email_template` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `template_code` varchar(50) NOT NULL COMMENT '模板编码（唯一）',
  `template_name` varchar(100) NOT NULL COMMENT '模板名称',
  `subject` varchar(200) NOT NULL COMMENT '邮件主题',
  `html_content` text COMMENT 'HTML模板内容',
  `text_content` text COMMENT '纯文本模板内容（备用）',
  `description` varchar(500) DEFAULT NULL COMMENT '模板描述',
  `is_active` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用：0-禁用 1-启用',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_template_code` (`template_code`),
  KEY `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邮件模板表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `feedback`
--

DROP TABLE IF EXISTS `feedback`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `feedback` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `content` text NOT NULL COMMENT '反馈内容',
  `email` varchar(100) DEFAULT NULL COMMENT '用户联系邮箱',
  `need_receipt` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否接收回执/允许联系',
  `contact_image` text COMMENT '图片凭证',
  `status` int(11) DEFAULT '0' COMMENT '处理状态:0-待处理,1-已处理',
  `admin_reply` text COMMENT '管理员回复',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_feedback_user` (`user_id`),
  KEY `idx_feedback_status` (`status`),
  CONSTRAINT `fk_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=6 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='意见反馈表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `invitation_feedback`
--

DROP TABLE IF EXISTS `invitation_feedback`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `invitation_feedback` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT 'user id',
  `status` enum('pending','interested','not_interested') NOT NULL DEFAULT 'pending' COMMENT 'feedback status',
  `intention_text` varchar(500) DEFAULT NULL COMMENT 'intention text max 500 chars',
  `conversation_status` enum('in_progress','accepted','rejected') DEFAULT NULL COMMENT 'conversation status',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_feedback_user` (`user_id`),
  KEY `idx_invitation_feedback_status` (`status`),
  KEY `idx_invitation_feedback_conversation_status` (`conversation_status`),
  CONSTRAINT `fk_invitation_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='campus super admin invitation feedback';
/*!40101 SET character_set_client = @saved_cs_client */;
--
-- Table structure for table `pending_invitation`
--

DROP TABLE IF EXISTS `pending_invitation`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `pending_invitation` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT 'user id',
  `invite_type` enum('SUPER_ADMIN') NOT NULL COMMENT 'invitation type',
  `expire_at` timestamp NOT NULL COMMENT 'expire time',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pending_invitation_user_type` (`user_id`,`invite_type`),
  KEY `idx_pending_invitation_expire_at` (`expire_at`),
  CONSTRAINT `fk_pending_invitation_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='pending invitation display flag';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `major`
--

DROP TABLE IF EXISTS `major`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `major` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `major_name` varchar(100) NOT NULL COMMENT '专业名称',
  `class_id` int(11) NOT NULL COMMENT '所属大类ID',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `fk_major_class` (`class_id`),
  CONSTRAINT `fk_major_class` FOREIGN KEY (`class_id`) REFERENCES `major_class` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB AUTO_INCREMENT=1037 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='专业表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `major_class`
--

DROP TABLE IF EXISTS `major_class`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `major_class` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `class_name` varchar(50) NOT NULL COMMENT '专业大类名称',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=113 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='专业大类表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `olive_branch_record`
--

DROP TABLE IF EXISTS `olive_branch_record`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `olive_branch_record` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `sender_id` int(11) NOT NULL COMMENT '发起人ID',
  `receiver_id` int(11) NOT NULL COMMENT '接收人ID(人才或队长)',
  `related_project_id` int(11) NOT NULL COMMENT '关联项目ID',
  `type` int(11) NOT NULL COMMENT '类型:1-人才互联,2-项目邀请(弃用)',
  `cost_type` int(11) NOT NULL COMMENT '消耗类型:1-免费额度,2-付费额度',
  `message` text COMMENT '邀请留言(已弃用)',
  `status` int(11) DEFAULT '0' COMMENT '状态:0-待处理,1-已接受,2-已拒绝,3-已忽略',
	`discussing_at` timestamp NULL DEFAULT NULL COMMENT '进入互相了解时间',
	`rejected_at` timestamp NULL DEFAULT NULL COMMENT '标记不合适时间',
	`admitted_at` timestamp NULL DEFAULT NULL COMMENT '同意入队时间',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_olive_sender` (`sender_id`),
  KEY `idx_olive_receiver` (`receiver_id`),
  KEY `idx_olive_project` (`related_project_id`),
  KEY `idx_olive_status` (`status`),
  CONSTRAINT `fk_olive_project` FOREIGN KEY (`related_project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_olive_receiver` FOREIGN KEY (`receiver_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_olive_sender` FOREIGN KEY (`sender_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=35 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='橄榄枝/联系记录表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `order`
--

DROP TABLE IF EXISTS `order`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `order` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `product_id` int(11) NOT NULL COMMENT '商品ID',
  `template_code` varchar(64) DEFAULT NULL COMMENT 'SMS template code',
  `template_name` varchar(100) DEFAULT NULL COMMENT 'SMS template display name',
  `price` decimal(10,2) NOT NULL COMMENT '下单时的单价快照',
  `quantity` int(11) NOT NULL COMMENT '数量',
  `actual_paid` decimal(10,2) NOT NULL COMMENT '实付金额',
  `status` int(11) DEFAULT '0' COMMENT '支付状态:0-待支付,1-已支付,2-已取消,3-已退款',
  `push_status` varchar(16) DEFAULT NULL COMMENT 'pending/success/failed',
  `push_retry_count` int(11) NOT NULL DEFAULT '0' COMMENT 'manual push retry count',
  `last_push_time` timestamp NULL DEFAULT NULL COMMENT 'last push attempt time',
  `push_error_message` varchar(500) DEFAULT NULL COMMENT 'last push failure reason',
  `delivery_scene` varchar(32) DEFAULT NULL COMMENT 'email_promotion/sms_notice',
  `delivery_payload` json DEFAULT NULL COMMENT 'immutable delivery context captured before payment',
  `wx_pay_no` varchar(100) DEFAULT NULL COMMENT '微信支付订单号',
  `out_trade_no` varchar(32) NOT NULL COMMENT '商户单号',
  `settlement_status` tinyint(4) DEFAULT '2' COMMENT 'settlement status:0-pending,1-settled,2-invalid,3-refunding',
  `refund_status` tinyint(4) DEFAULT '0' COMMENT 'refund status:0-none,1-pending,2-success,3-rejected,4-withdrawn',
  `refund_applicant_type` tinyint(4) DEFAULT NULL COMMENT 'refund applicant:0-consumer,1-school super admin',
  `refund_applicant_admin_id` int(11) DEFAULT NULL COMMENT 'school super admin who applied refund',
  `refund_reason` text COMMENT 'refund reason',
  `reject_reason` varchar(500) DEFAULT NULL COMMENT 'refund reject reason',
  `refund_apply_time` timestamp NULL DEFAULT NULL COMMENT 'refund apply time',
  `refund_handle_time` timestamp NULL DEFAULT NULL COMMENT 'refund handle time',
  `reject_time` timestamp NULL DEFAULT NULL COMMENT 'refund reject time',
  `refund_withdraw_time` timestamp NULL DEFAULT NULL COMMENT 'refund withdraw time',
  `refund_operator_admin_id` int(11) DEFAULT NULL COMMENT 'refund operator admin id',
  `settlement_batch_no` varchar(64) DEFAULT NULL COMMENT 'settlement batch no',
  `settlement_time` timestamp NULL DEFAULT NULL COMMENT 'settlement time',
  `settlement_operator_admin_id` int(11) DEFAULT NULL COMMENT 'settlement operator admin id',
  `pay_time` timestamp NULL DEFAULT NULL COMMENT '支付时间',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_wx_pay_no` (`wx_pay_no`),
  KEY `idx_order_settlement_status` (`settlement_status`),
  KEY `idx_order_refund_status` (`refund_status`),
  KEY `idx_order_delivery_recovery` (`status`,`push_status`,`delivery_scene`,`updated_at`),
  KEY `fk_order_merged_user` (`user_id`),
  KEY `fk_order_merged_product` (`product_id`),
  CONSTRAINT `fk_order_merged_product` FOREIGN KEY (`product_id`) REFERENCES `product` (`id`) ON DELETE RESTRICT,
  CONSTRAINT `fk_order_merged_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=4 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单总表(合并主表与详情)';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `product`
--

DROP TABLE IF EXISTS `product`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `product` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `name` varchar(100) NOT NULL COMMENT '商品名称',
  `type` int(11) NOT NULL COMMENT '类型:1-虚拟币,2-服务权益',
  `description` text COMMENT '商品描述',
  `price` decimal(10,2) NOT NULL COMMENT '商品价格',
  `config_json` json DEFAULT NULL COMMENT '配置参数(如增加多少个橄榄枝)',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=7 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='商品表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `project`
--

DROP TABLE IF EXISTS `project`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `project` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `creator_id` int(11) NOT NULL COMMENT '队长(用户ID)',
  `name` varchar(200) NOT NULL COMMENT '项目名称',
  `description` text COMMENT '项目详情',
  `school_id` int(11) DEFAULT NULL COMMENT '所属学校',
  `direction` int(11) DEFAULT NULL COMMENT '项目方向:1-落地,2-比赛,3-学习',
  `member_count` int(11) DEFAULT NULL COMMENT '需求人数',
  `status` int(11) DEFAULT '0' COMMENT '项目状态:0-待审核,1-已通过,2-已驳回,3-完成招募,4-删除中,5-已结束',
  `promotion_status` int(11) DEFAULT '0' COMMENT '推广状态:0-无,1-推广中,2-已结束',
  `promotion_expire_time` timestamp NULL DEFAULT NULL COMMENT '推广结束时间',
  `view_count` int(11) DEFAULT '0' COMMENT '浏览量',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `reject_reason` varchar(255) DEFAULT NULL COMMENT '驳回原因',
  `passive_status_changed_at` timestamp NULL DEFAULT NULL COMMENT '待审核项目被动审核为通过/拒绝的时间，用于我的项目红点',
  `recruit_completed_at` timestamp NULL DEFAULT NULL COMMENT 'Recruit completed timestamp',
  `ended_at` timestamp NULL DEFAULT NULL COMMENT 'Project ended timestamp',
  `deleted_at` timestamp NULL DEFAULT NULL COMMENT '删除时间',
  `admin_note` text COMMENT '管理员跟进备注',
  `admin_note_updated_at` timestamp NULL DEFAULT NULL COMMENT '管理员备注更新时间',
  `is_cross_school` tinyint(4) DEFAULT '1' COMMENT '是否跨校: 1-可以,0-不可以',
  `education_requirement` tinyint(4) DEFAULT '1' COMMENT '学历要求1-大专2-本科',
  `skill_requirement` text COMMENT '技能要求',
  `publisher_role` varchar(32) DEFAULT NULL COMMENT 'publisher project role',
  `initiating_school_id` int(11) DEFAULT NULL COMMENT 'initiating school ID',
  PRIMARY KEY (`id`),
  KEY `idx_project_creator` (`creator_id`),
  KEY `idx_project_school` (`school_id`),
  KEY `idx_project_school_status` (`school_id`,`status`,`id`),
  KEY `idx_project_status` (`status`),
  KEY `idx_project_created` (`created_at`),
  CONSTRAINT `fk_project_creator` FOREIGN KEY (`creator_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_project_school` FOREIGN KEY (`school_id`) REFERENCES `school` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=342 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='项目表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `project_application`
--

DROP TABLE IF EXISTS `project_application`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `project_application` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `project_id` int(11) NOT NULL COMMENT '项目ID',
  `user_id` int(11) NOT NULL COMMENT '申请人',
  `apply_reason` text COMMENT '申请理由/留言',
  `contact` text COMMENT '联系方式',
  `status` int(11) DEFAULT '0' COMMENT '状态:0-待审核,1-已通过,2-已拒绝',
  `reply_msg` text COMMENT '队长回复',
  `is_read` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'reviewer read status',
  `reviewer_id` bigint unsigned DEFAULT NULL COMMENT 'reviewer user ID',
  `reviewer_role` varchar(32) DEFAULT NULL COMMENT 'reviewer project role',
  `assigned_role` varchar(32) DEFAULT NULL COMMENT 'assigned project role',
  `applied_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '申请时间',
  `discussing_at` timestamp NULL DEFAULT NULL COMMENT 'Discussing status timestamp',
  `rejected_at` timestamp NULL DEFAULT NULL COMMENT 'Rejected timestamp',
  `joined_at` timestamp NULL DEFAULT NULL COMMENT 'Joined/admitted timestamp',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_user` (`project_id`,`user_id`),
  KEY `idx_application_project` (`project_id`),
  KEY `idx_application_user` (`user_id`),
  KEY `idx_application_status` (`status`),
  KEY `idx_pa_user_updated_status` (`user_id`,`updated_at`,`status`),
  KEY `idx_pa_status_project` (`status`,`project_id`),
  KEY `idx_project_application_reviewer` (`reviewer_id`),
  KEY `idx_project_application_reviewer_role` (`reviewer_role`),
  KEY `idx_project_application_assigned_role` (`assigned_role`),
  CONSTRAINT `fk_app_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_app_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=562 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='项目申请表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `school`
--

DROP TABLE IF EXISTS `school`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `school` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `school_name` varchar(100) NOT NULL COMMENT '学校名称',
  `school_code` varchar(50) DEFAULT NULL COMMENT '学校代码',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `province` varchar(100) DEFAULT NULL COMMENT '学校所处省份',
  PRIMARY KEY (`id`),
  UNIQUE KEY `school_code` (`school_code`)
) ENGINE=InnoDB AUTO_INCREMENT=2979 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='学校字典表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `subscribe`
--

DROP TABLE IF EXISTS `subscribe`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `subscribe` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `status` tinyint(4) DEFAULT '1' COMMENT '状态（0-允许/1-拒绝/2-总是保持）',
  `biz_key` varchar(100) NOT NULL COMMENT '业务标识',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_biz` (`user_id`, `biz_key`),
  KEY `idx_subscribe_user` (`user_id`),
  KEY `idx_subscribe_biz_status_user` (`biz_key`, `status`, `user_id`),
  CONSTRAINT `fk_sub_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='消息订阅配置表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `msg_template_config`
--

DROP TABLE IF EXISTS `msg_template_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `msg_template_config` (
  `biz_key` varchar(50) NOT NULL COMMENT '业务标识',
  `template_id` varchar(100) NOT NULL COMMENT '微信模板ID',
  `template_title` varchar(100) DEFAULT NULL COMMENT '模板标题',
  `content_json` json NOT NULL COMMENT '字段映射配置',
  `page_path` varchar(255) DEFAULT NULL COMMENT '点击订阅消息卡片后跳转的小程序页面路径',
  `remark` varchar(20) DEFAULT NULL COMMENT '模板字段备注（最多20字符）',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用',
  `platform_status` varchar(32) DEFAULT NULL COMMENT '微信平台核验状态',
  `platform_verified_at` datetime DEFAULT NULL COMMENT '最近微信平台核验时间',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`biz_key`),
  UNIQUE KEY `uk_msg_template_template_id` (`template_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订阅消息模板配置表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `wx_subscribe_delivery`
--

DROP TABLE IF EXISTS `wx_subscribe_delivery`;
CREATE TABLE `wx_subscribe_delivery` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `biz_key` varchar(100) NOT NULL,
  `template_id` varchar(100) DEFAULT NULL,
  `business_data` json NOT NULL,
  `page_path` varchar(255) DEFAULT NULL,
  `status` varchar(20) NOT NULL DEFAULT 'PENDING',
  `attempt_count` int NOT NULL DEFAULT '0',
  `next_attempt_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `claimed_at` datetime DEFAULT NULL,
  `sent_at` datetime DEFAULT NULL,
  `last_errcode` int DEFAULT NULL,
  `last_errmsg` varchar(1000) DEFAULT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_wx_subscribe_due` (`status`, `next_attempt_at`, `id`),
  KEY `idx_wx_subscribe_created_status` (`created_at`, `status`),
  KEY `idx_wx_subscribe_user_time` (`user_id`, `created_at`),
  KEY `idx_wx_subscribe_biz_time` (`biz_key`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信订阅消息可靠投递与审计日志';

--
-- Table structure for table `wx_subscribe_status_history`
--

DROP TABLE IF EXISTS `wx_subscribe_status_history`;
CREATE TABLE `wx_subscribe_status_history` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `biz_key` varchar(100) NOT NULL,
  `template_id` varchar(100) NOT NULL,
  `result` varchar(20) NOT NULL,
  `status` tinyint NOT NULL,
  `source` varchar(32) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_wx_sub_history_user_biz_time` (`user_id`, `biz_key`, `created_at`),
  KEY `idx_wx_sub_history_biz_time` (`biz_key`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信订阅授权状态变更历史';

--
-- Table structure for table `roadmap`
--

DROP TABLE IF EXISTS `roadmap`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `roadmap` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL COMMENT '发布日期',
  `title` varchar(100) NOT NULL COMMENT '标题',
  `content` text NOT NULL COMMENT '详细内容',
  `link` varchar(500) DEFAULT NULL COMMENT '公众号链接',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_roadmap_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='平台进度看板';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_roadmap_view`
--

DROP TABLE IF EXISTS `user_roadmap_view`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_roadmap_view` (
  `user_id` int(10) unsigned NOT NULL,
  `last_viewed_at` timestamp NULL DEFAULT NULL COMMENT '最后查看时间，NULL代表从未查看',
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户进度看板已读时间';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `talent_profile`
--

DROP TABLE IF EXISTS `talent_profile`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `talent_profile` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '关联用户ID',
  `self_evaluation` text COMMENT '自我评价',
  `skill_summary` json COMMENT '技能标签',
  `project_experience` text COMMENT '项目经历',
  `mbti` varchar(10) DEFAULT NULL COMMENT 'MBTI性格类型',
  `status` int(11) DEFAULT '1' COMMENT '状态:1-上架,0-下架',
  `reject_reason` varchar(500) DEFAULT NULL COMMENT '驳回/下架原因',
  `is_public_contact` tinyint(1) DEFAULT '0' COMMENT '是否公开联系方式',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_id` (`user_id`),
  KEY `idx_talent_status` (`status`),
  CONSTRAINT `fk_talent_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=46 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='人才档案表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user`
--

DROP TABLE IF EXISTS `user`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `openid` varchar(100) NOT NULL COMMENT '微信OpenID',
  `nickname` varchar(50) DEFAULT NULL COMMENT '昵称',
  `phone` varchar(20) DEFAULT NULL COMMENT '手机号',
  `email` varchar(100) DEFAULT NULL COMMENT '邮箱',
  `school_id` int(11) DEFAULT NULL COMMENT '学校ID',
  `major_id` int(11) DEFAULT NULL COMMENT '专业ID',
  `grade` int(11) DEFAULT NULL COMMENT '年级',
  `olive_branch_count` int(11) DEFAULT '0' COMMENT '付费橄榄枝余额',
  `free_branch_used_today` int(11) DEFAULT '0' COMMENT '今日已用免费次数(每日重置)',
  `last_active_date` date DEFAULT NULL COMMENT '最后活跃日期(用于重置免费次数)',
  `auth_status` int(11) DEFAULT '0' COMMENT '认证状态:0-未认证,1-已认证,2-认证失败',
  `auth_img_url` text COMMENT '学生证认证图',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `email_opt_out` tinyint(1) DEFAULT '0' COMMENT '是否退订邮件推广',
  `avatar_url` varchar(100) DEFAULT NULL COMMENT '用户头像url',
  `cover_image` varchar(100) DEFAULT NULL COMMENT '用户中心封面url',
  `unionid` varchar(100) DEFAULT NULL COMMENT '微信用户unionid',
  `wechat_id` varchar(100) DEFAULT NULL COMMENT '微信号',
  `sent_olive_viewed_at` timestamp NULL DEFAULT NULL COMMENT '最后查看已发送橄榄枝时间',
  `applications_last_viewed_at` timestamp NULL DEFAULT NULL COMMENT '最后查看投递管理时间',
  `last_viewed_my_projects_at` timestamp NULL DEFAULT NULL COMMENT '最后查看我的项目页时间',
  `user_status` tinyint(4) NOT NULL DEFAULT '0' COMMENT '0-normal,1-banned,2-graduated',
  `ban_reason` varchar(500) DEFAULT NULL COMMENT 'account ban reason',
  `collaboration_score` DECIMAL(5,2) NOT NULL DEFAULT 90.00 COMMENT 'collaboration score, 0-100',
  PRIMARY KEY (`id`),
  UNIQUE KEY `openid` (`openid`),
  UNIQUE KEY `uq_user_phone` (`phone`),
  UNIQUE KEY `uq_user_email` (`email`),
  KEY `idx_user_school` (`school_id`),
  KEY `idx_user_school_auth` (`school_id`,`auth_status`,`id`),
  KEY `idx_user_major` (`major_id`),
  CONSTRAINT `fk_user_major` FOREIGN KEY (`major_id`) REFERENCES `major` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_user_school` FOREIGN KEY (`school_id`) REFERENCES `school` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=2153 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for project roles, membership cycles, and periodic ratings
--

DROP TABLE IF EXISTS `project_member_score`;
DROP TABLE IF EXISTS `project_member_rating`;
DROP TABLE IF EXISTS `project_members`;
DROP TABLE IF EXISTS `project_role`;
CREATE TABLE `project_role` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(32) NOT NULL,
  `name` VARCHAR(32) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `sort_order` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_role_code` (`code`),
  KEY `idx_project_role_status_sort` (`status`,`sort_order`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目团队角色字典';

INSERT INTO `project_role` (`code`,`name`,`status`,`sort_order`) VALUES
  ('TEAM_LEADER','团队负责人',1,10),
  ('TECH_LEADER','技术负责人',1,20),
  ('OPERATIONS_LEADER','运营负责人',1,30),
  ('PUBLICITY_LEADER','宣传负责人',1,40),
  ('RECRUITMENT_LEADER','招募负责人',1,50),
  ('DESIGN_LEADER','美化负责人',1,60),
  ('LEGAL_LEADER','法务负责人',1,70),
  ('TEAM_MEMBER','团队成员',1,80),
  ('LEARNING_MEMBER','学习成员',1,90);

CREATE TABLE `project_members` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `role` VARCHAR(32) NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_members_project_user` (`project_id`,`user_id`),
  KEY `idx_project_members_user` (`user_id`),
  KEY `idx_project_members_project_role` (`project_id`,`role`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目团队成员关系';

INSERT INTO `project_members` (`project_id`,`user_id`,`role`,`created_at`,`updated_at`)
SELECT p.id,p.creator_id,'TEAM_LEADER',NOW(),NOW()
FROM `project` p
WHERE p.creator_id IS NOT NULL;

CREATE TABLE `project_member_rating` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` BIGINT UNSIGNED NOT NULL,
  `rater_id` BIGINT UNSIGNED NOT NULL COMMENT '评分用户 ID',
  `target_id` BIGINT UNSIGNED NOT NULL COMMENT '被评分用户 ID',
  `rater_member_id` BIGINT UNSIGNED NOT NULL COMMENT '评分人的本次成员关系 ID',
  `target_member_id` BIGINT UNSIGNED NOT NULL COMMENT '被评分人的本次成员关系 ID',
  `rater_role` VARCHAR(32) NOT NULL COMMENT '评分提交时的角色快照',
  `rater_weight` DECIMAL(3,2) NOT NULL COMMENT '评分提交时的角色权重快照',
  `score` TINYINT UNSIGNED NOT NULL COMMENT '0-100',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_rating_target_latest` (`target_member_id`,`rater_id`,`id`),
  KEY `idx_rating_cooldown` (`project_id`,`rater_id`,`target_member_id`,`created_at`),
  KEY `idx_rating_project_target` (`project_id`,`target_id`,`created_at`),
  KEY `idx_rating_target_history` (`target_id`,`created_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目成员周期性互评明细';

CREATE TABLE `project_member_score` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` BIGINT UNSIGNED NOT NULL,
  `project_member_id` BIGINT UNSIGNED NOT NULL COMMENT '成员本次加入关系 ID',
  `member_id` BIGINT UNSIGNED NOT NULL COMMENT '成员用户 ID',
  `score` DECIMAL(5,2) DEFAULT NULL COMMENT '当前角色加权平均分，NULL 表示暂无评分',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_member_score_cycle` (`project_member_id`),
  KEY `idx_project_member_score_lookup` (`project_id`,`member_id`,`project_member_id`),
  KEY `idx_member_score_user_project` (`member_id`,`project_id`,`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目成员当前生效评分';
--
-- Table structure for table `user_competition_group`
--

DROP TABLE IF EXISTS `user_competition_group`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_competition_group` (
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `status` varchar(20) DEFAULT NULL COMMENT '比赛交流群状态：entered/rejected',
  `note` text COMMENT '比赛交流群备注',
  `updated_by_admin_id` int(11) DEFAULT NULL COMMENT '最后更新管理员ID',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`user_id`),
  KEY `idx_user_competition_group_status` (`status`),
  KEY `idx_user_competition_group_updated_by` (`updated_by_admin_id`),
  CONSTRAINT `fk_user_competition_group_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_competition_group_admin` FOREIGN KEY (`updated_by_admin_id`) REFERENCES `admin_user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户比赛交流群状态';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `collaboration_score`
--

DROP TABLE IF EXISTS `status_notification`;
DROP TABLE IF EXISTS `project_member_removal`;
CREATE TABLE `project_member_removal` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL,
  `project_id` INT NOT NULL,
  `operator_id` INT NOT NULL,
  `role` VARCHAR(32) NOT NULL,
  `joined_at` DATETIME NOT NULL,
  `removed_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `score` DECIMAL(5,2) NULL COMMENT '项目成员移除时固化的最终评分',
  PRIMARY KEY (`id`),
  KEY `idx_member_removal_user` (`user_id`,`removed_at`),
  KEY `idx_member_removal_project` (`project_id`),
  KEY `idx_member_removal_user_project_cycle` (`user_id`,`project_id`,`joined_at`,`removed_at`),
  CONSTRAINT `fk_member_removal_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_member_removal_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Removed project member snapshots';
CREATE TABLE `status_notification` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL,
  `type` VARCHAR(50) NOT NULL,
	`application_id` INT NULL,
	`olive_branch_id` INT NULL,
	`member_removal_id` BIGINT NULL,
	`priority` INT NOT NULL DEFAULT 10,
  `displayed_at` TIMESTAMP NULL DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_status_notification_pending` (`user_id`, `displayed_at`, `id`),
  KEY `idx_status_notification_application` (`application_id`),
	KEY `idx_status_notification_olive` (`olive_branch_id`),
	KEY `idx_status_notification_member_removal` (`member_removal_id`),
  CONSTRAINT `fk_status_notification_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
	CONSTRAINT `fk_status_notification_application` FOREIGN KEY (`application_id`) REFERENCES `project_application` (`id`) ON DELETE CASCADE,
	CONSTRAINT `fk_status_notification_olive` FOREIGN KEY (`olive_branch_id`) REFERENCES `olive_branch_record` (`id`) ON DELETE CASCADE,
	CONSTRAINT `fk_status_notification_member_removal` FOREIGN KEY (`member_removal_id`) REFERENCES `project_member_removal` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Status notification delivery queue';

DROP TABLE IF EXISTS `collaboration_score`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `collaboration_score` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` INT UNSIGNED NOT NULL COMMENT 'rated user ID',
  `project_id` INT UNSIGNED NOT NULL COMMENT 'project ID',
  `scorer_id` INT UNSIGNED NOT NULL COMMENT 'scorer user ID',
  `score` DECIMAL(5,2) NOT NULL COMMENT '项目成员移除时固化的最终评分',
  `rating_count` INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '固化评分包含的有效评分人数',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_collaboration_score_user` (`user_id`),
  KEY `idx_collaboration_score_project` (`project_id`),
  KEY `idx_collaboration_score_scorer` (`scorer_id`),
  KEY `idx_collaboration_score_user_created` (`user_id`,`created_at`),
  KEY `idx_collaboration_score_user_project_created` (`user_id`,`project_id`,`created_at`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='collaboration score history';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping routines for database 'lianxi'
--
SET @@SESSION.SQL_LOG_BIN = @MYSQLDUMP_TEMP_LOG_BIN;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2026-03-14 11:40:30
