-- MySQL 5.7. Run against the message-center database.
INSERT INTO email_template
  (channel, template_code, external_template_code, sign_name, template_name, subject, html_content, text_content, description, is_active, created_at, updated_at)
VALUES
  ('SMS', 'OLIVE_BRANCH_REJECTED', 'SMS_508700270', '泉州守途科技', '橄榄枝不合适', '', NULL,
   '亲爱的${nickname}同学，你好！ 我是${projectTitle}的${teamRole}，很欣赏你积极进取的态度，并在此前向你发出橄榄枝。 但很抱歉，经团队认真商议，综合评估后认为你与项目现阶段的需求暂时不太契合。我们会保存橄榄枝记录，今后如有合适机会，我们会第一时间向你再次发送橄榄枝，也欢迎继续关注我们。 再次感谢你对我们橄榄枝的回应，祝你学业顺利，生活愉快！ 快组校园小程序 敬上',
   '橄榄枝不合适结果通知；变量 nickname、projectTitle、teamRole。', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
  ('SMS', 'OLIVE_BRANCH_ACCEPTED', 'SMS_508565285', '泉州守途科技', '橄榄枝录用祝贺', '', NULL,
   '亲爱的${nickname}同学，你好！ 我是${projectTitle}的${teamRole}，感谢你对我们发出的橄榄枝的回应并耐心地与我们交流。经过综合评估，我们很高兴地通知你，你已成为我们团队的一员。关于具体角色等信息，你可打开快组校园小程序查看详情。',
   '橄榄枝录用结果通知；变量 nickname、projectTitle、teamRole。', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  external_template_code=VALUES(external_template_code), sign_name=VALUES(sign_name),
  template_name=VALUES(template_name), text_content=VALUES(text_content),
  description=VALUES(description), is_active=1, updated_at=CURRENT_TIMESTAMP;
