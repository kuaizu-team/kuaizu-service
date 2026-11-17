-- 日程分享、共同日程与团队忙闲；依赖此前全部 user_schedule 迁移。执行前备份，只执行一次。
ALTER TABLE user_schedule
 ADD COLUMN joint_id BIGINT NULL,
 ADD UNIQUE KEY uk_schedule_joint_user (user_id,joint_id),
 ADD KEY idx_schedule_joint (joint_id);
CREATE TABLE schedule_privacy (
 user_id INT PRIMARY KEY,
 team_details TINYINT(1) NOT NULL DEFAULT 0,
 member_contacts TINYINT(1) NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE schedule_share (
 id BIGINT AUTO_INCREMENT PRIMARY KEY,
 token CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 owner_id INT NOT NULL,
 kind VARCHAR(10) NOT NULL,
 share_date VARCHAR(10) NOT NULL DEFAULT '',
 payload MEDIUMTEXT NOT NULL,
 revoked TINYINT(1) NOT NULL DEFAULT 0,
 expires_at DATETIME NOT NULL,
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE KEY uk_schedule_share_token(token),
 KEY idx_schedule_share_owner(owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE schedule_share_copy (
 share_id BIGINT NOT NULL,
 user_id INT NOT NULL,
 schedule_id BIGINT NOT NULL,
 PRIMARY KEY(share_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE schedule_joint (
 id BIGINT AUTO_INCREMENT PRIMARY KEY,
 creator_id INT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE KEY uk_schedule_joint_request(creator_id,request_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE schedule_notice (
 id BIGINT AUTO_INCREMENT PRIMARY KEY,
 user_id INT NOT NULL,
 joint_id BIGINT NOT NULL,
 message VARCHAR(240) NOT NULL,
 is_read TINYINT(1) NOT NULL DEFAULT 0,
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 KEY idx_schedule_notice_user(user_id,is_read,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
SHOW COLUMNS FROM user_schedule;
SHOW TABLES LIKE 'schedule_%';
-- 顺序：SQL → 后端 → 小程序。默认团队仅忙闲、联系方式不开放。此脚本不发送任何消息。
