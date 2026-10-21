-- MySQL 8.0.13. Review and run manually AFTER the cooperation schema migration.
-- Prepared only; this file has NOT been executed by the agent.
-- Initializes one business template DISABLED. No WeChat messages are sent.
-- Re-running preserves an existing matching template and its enabled state.
-- Conflicting key/ID/mapping aborts; never reassigns another business template.

DROP PROCEDURE IF EXISTS configure_campus_cooperation_template;
DELIMITER $$
CREATE PROCEDURE configure_campus_cooperation_template()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.TABLES
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='campus_cooperation_application') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Run the cooperation schema migration first';
  END IF;

  IF EXISTS (SELECT 1 FROM msg_template_config
      WHERE template_id='PiZRtGMWnm2e5oi9KGqjtNE78ORAXJLFf4gJk_TJeEA'
        AND biz_key<>'MSG_CAMPUS_COOPERATION_RESULT') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Template ID belongs to another business key; stop for review';
  END IF;

  IF EXISTS (SELECT 1 FROM msg_template_config
      WHERE biz_key='MSG_CAMPUS_COOPERATION_RESULT'
        AND template_id<>'PiZRtGMWnm2e5oi9KGqjtNE78ORAXJLFf4gJk_TJeEA') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Cooperation business key already uses a different template ID';
  END IF;

  IF EXISTS (SELECT 1 FROM msg_template_config
      WHERE biz_key='MSG_CAMPUS_COOPERATION_RESULT'
        AND (COALESCE(JSON_TYPE(content_json),'')<>'OBJECT'
          OR COALESCE(JSON_LENGTH(content_json),0)<>3
          OR COALESCE(JSON_UNQUOTE(JSON_EXTRACT(content_json,'$.application')),'')<>'thing10'
          OR COALESCE(JSON_UNQUOTE(JSON_EXTRACT(content_json,'$.result')),'')<>'phrase3'
          OR COALESCE(JSON_UNQUOTE(JSON_EXTRACT(content_json,'$.remark')),'')<>'thing8')) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Existing cooperation field mapping differs; stop for review';
  END IF;

  IF NOT EXISTS (SELECT 1 FROM msg_template_config WHERE biz_key='MSG_CAMPUS_COOPERATION_RESULT') THEN
    INSERT INTO msg_template_config
      (biz_key,template_id,template_title,content_json,remark,page_path,enabled,platform_status,platform_verified_at)
    VALUES
      ('MSG_CAMPUS_COOPERATION_RESULT',
       'PiZRtGMWnm2e5oi9KGqjtNE78ORAXJLFf4gJk_TJeEA',
       '申请结果通知',
       JSON_OBJECT('application','thing10','result','phrase3','remark','thing8'),
       '点击查看联系方式',NULL,0,NULL,NULL);
  END IF;
END$$
DELIMITER ;
CALL configure_campus_cooperation_template();
DROP PROCEDURE configure_campus_cooperation_template;

-- Return this row for review. First initialization should show enabled=0.
-- The page containing a cooperation ID is supplied by the transactional outbox.
SELECT biz_key,template_id,template_title,content_json,remark,page_path,enabled,platform_status,platform_verified_at
FROM msg_template_config WHERE biz_key='MSG_CAMPUS_COOPERATION_RESULT';
