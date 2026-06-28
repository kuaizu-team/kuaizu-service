-- 被拒安慰短信模板 v2（MySQL 5.7）
-- 仅当当前记录仍指向 v1 阿里云模板时更新，避免覆盖并发或后续配置。
START TRANSACTION;

UPDATE `email_template`
SET
    `external_template_code` = 'SMS_508690285',
    `template_name` = '被拒安慰v2',
    `text_content` = '亲爱的${nickname}同学，你好！ 我是${projectTitle}的${teamRole}，感谢你将名片投递给我们，我们很欣赏你积极进取的态度。 很遗憾，经过团队认真商议，我们认为你与项目现阶段的需求暂时不太契合。你的名片已保存至项目人才库，今后如有合适机会，我们会第一时间向你发送橄榄枝，也欢迎你继续关注我们。 再次感谢你对我们的关注和支持，祝你学业顺利，生活愉快！ 快组校园小程序 敬上',
    `updated_at` = CURRENT_TIMESTAMP
WHERE `channel` = 'SMS'
  AND `template_code` = 'PROJECT_APPLICATION_REJECTED'
  AND `external_template_code` = 'SMS_508685265'
  AND `is_active` = 1;

SELECT ROW_COUNT() AS `updated_rows`;

COMMIT;

-- 执行后核验（应仅返回 1 行，且三个变量保持不变）：
SELECT
    `id`,
    `channel`,
    `template_code`,
    `external_template_code`,
    `template_name`,
    `text_content`,
    `is_active`,
    `updated_at`
FROM `email_template`
WHERE `channel` = 'SMS'
  AND `template_code` = 'PROJECT_APPLICATION_REJECTED';
