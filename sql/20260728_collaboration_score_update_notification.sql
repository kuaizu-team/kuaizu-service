-- 协作指数更新提醒订阅消息模板配置（MySQL 5.7）
INSERT INTO `msg_template_config` (`biz_key`, `template_id`, `template_title`, `content_json`, `page_path`)
VALUES (
  'MSG_COLLABORATION_SCORE_UPDATE',
  'qM1amcWctESoNO99z3OB6zT2gAderjR0HPEI1uGAUKU',
  '协作指数更新提醒',
  JSON_OBJECT('score', 'number1', 'updated_at', 'time2', 'remark', 'thing3'),
  'pages/profile/profile'
)
ON DUPLICATE KEY UPDATE
  `template_id` = VALUES(`template_id`),
  `template_title` = VALUES(`template_title`),
  `content_json` = VALUES(`content_json`),
  `page_path` = VALUES(`page_path`),
  `updated_at` = CURRENT_TIMESTAMP;
