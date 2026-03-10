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

SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '09c0ed11-3a14-11f0-9fc2-00163e0c6f4a:1-15924';

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
  `status` tinyint(4) DEFAULT '1' COMMENT '状态:1-启用,0-禁用',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `username` (`username`),
  KEY `idx_admin_user_username` (`username`)
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
  `order_id` int(11) NOT NULL COMMENT '关联订单',
  `project_id` int(11) DEFAULT NULL COMMENT '推广的项目',
  `creator_id` int(11) NOT NULL COMMENT '发起人（队长）',
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
  CONSTRAINT `fk_email_promotion_order` FOREIGN KEY (`order_id`) REFERENCES `order` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_email_promotion_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=5 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='邮件推广记录表';
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
) ENGINE=InnoDB AUTO_INCREMENT=34 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='橄榄枝/联系记录表';
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
  `actual_paid` decimal(10,2) NOT NULL COMMENT '实付金额',
  `status` int(11) DEFAULT '0' COMMENT '支付状态:0-待支付,1-已支付,2-已取消,3-已退款',
  `wx_pay_no` varchar(100) DEFAULT NULL COMMENT '微信支付订单号',
  `pay_time` timestamp NULL DEFAULT NULL COMMENT '支付时间',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `unq_order_wx_pay_no` (`wx_pay_no`),
  KEY `fk_order_user` (`user_id`),
  CONSTRAINT `fk_order_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=20 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单表';
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `order_item`
--

DROP TABLE IF EXISTS `order_item`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `order_item` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `order_id` int(11) NOT NULL COMMENT '订单ID',
  `product_id` int(11) NOT NULL COMMENT '商品ID',
  `price` decimal(10,2) NOT NULL COMMENT '下单时的单价快照',
  `quantity` int(11) NOT NULL COMMENT '数量',
  PRIMARY KEY (`id`),
  KEY `fk_order_item_order` (`order_id`),
  KEY `fk_order_item_product` (`product_id`),
  CONSTRAINT `fk_order_item_order` FOREIGN KEY (`order_id`) REFERENCES `order` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_order_item_product` FOREIGN KEY (`product_id`) REFERENCES `product` (`id`) ON DELETE RESTRICT
) ENGINE=InnoDB AUTO_INCREMENT=19 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='订单详情表';
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
  `config_json` text COMMENT '配置参数(如增加多少个橄榄枝)',
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
  `status` int(11) DEFAULT '0' COMMENT '审核状态:0-待审核,1-已通过,2-已驳回',
  `promotion_status` int(11) DEFAULT '0' COMMENT '推广状态:0-无,1-推广中,2-已结束',
  `promotion_expire_time` timestamp NULL DEFAULT NULL COMMENT '推广结束时间',
  `view_count` int(11) DEFAULT '0' COMMENT '浏览量',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `is_cross_school` tinyint(4) DEFAULT '1' COMMENT '是否跨校: 1-可以,2-不可以',
  `education_requirement` tinyint(4) DEFAULT '1' COMMENT '学历要求1-大专2-本科',
  `skill_requirement` text COMMENT '技能要求',
  PRIMARY KEY (`id`),
  KEY `idx_project_creator` (`creator_id`),
  KEY `idx_project_school` (`school_id`),
  KEY `idx_project_status` (`status`),
  KEY `idx_project_created` (`created_at`),
  CONSTRAINT `fk_project_creator` FOREIGN KEY (`creator_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_project_school` FOREIGN KEY (`school_id`) REFERENCES `school` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=336 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='项目表';
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
  `apply_reason` text COMMENT '申请理由/留言(已弃用)',
  `contact` text COMMENT '联系方式',
  `status` int(11) DEFAULT '0' COMMENT '状态:0-待审核,1-已通过,2-已拒绝',
  `reply_msg` text COMMENT '队长回复(已弃用)',
  `applied_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '申请时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_user` (`project_id`,`user_id`),
  KEY `idx_application_project` (`project_id`),
  KEY `idx_application_user` (`user_id`),
  KEY `idx_application_status` (`status`),
  CONSTRAINT `fk_app_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_app_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB AUTO_INCREMENT=556 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='项目申请表';
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
-- Table structure for table `subscribe_config`
--

DROP TABLE IF EXISTS `subscribe_config`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `subscribe_config` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `target_type` varchar(50) NOT NULL COMMENT '订阅类型(新项目/审核结果/投递进度)',
  `filter_json` text COMMENT '过滤条件(如:只看某学校项目)',
  `email` varchar(100) DEFAULT NULL COMMENT '接收邮箱',
  `is_active` tinyint(1) DEFAULT '1' COMMENT '是否开启',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_subscribe_user` (`user_id`),
  KEY `idx_subscribe_active` (`is_active`),
  CONSTRAINT `fk_sub_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='消息订阅配置表';
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
  `skill_summary` text COMMENT '技能标签',
  `project_experience` text COMMENT '项目经历',
  `mbti` varchar(10) DEFAULT NULL COMMENT 'MBTI性格类型',
  `status` int(11) DEFAULT '1' COMMENT '状态:1-上架,0-下架',
  `is_public_contact` tinyint(1) DEFAULT '0' COMMENT '是否公开联系方式',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `user_id` (`user_id`),
  KEY `idx_talent_user` (`user_id`),
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
  PRIMARY KEY (`id`),
  UNIQUE KEY `openid` (`openid`),
  KEY `idx_user_openid` (`openid`),
  KEY `idx_user_school` (`school_id`),
  KEY `idx_user_major` (`major_id`),
  CONSTRAINT `fk_user_major` FOREIGN KEY (`major_id`) REFERENCES `major` (`id`) ON DELETE SET NULL,
  CONSTRAINT `fk_user_school` FOREIGN KEY (`school_id`) REFERENCES `school` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB AUTO_INCREMENT=2151 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户表';
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

-- Dump completed on 2026-02-25 14:13:48
