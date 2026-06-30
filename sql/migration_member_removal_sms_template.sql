-- MySQL 5.7. Run against the message-center database.
INSERT INTO email_template
  (channel, template_code, external_template_code, sign_name, template_name, subject, html_content, text_content, description, is_active, created_at, updated_at)
VALUES
  ('SMS', 'MEMBER_REMOVAL_THANKS', 'SMS_508385290', '泉州守途科技', '移除成员短信', '', NULL,
   '亲爱的${nickname}同学： 感谢你曾经作为${projectTitle}的${teamRole}付出的努力。在项目中，我们曾携手同行，共同为一个目标而奋进。这段旅程里，我们为彼此和合作伙伴创造了更大的价值。祝愿你未来一切顺利！ 快组校园小程序 敬上',
   '成员移除后的协作感谢短信；变量 nickname、projectTitle、teamRole。', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
  external_template_code=VALUES(external_template_code), sign_name=VALUES(sign_name),
  template_name=VALUES(template_name), text_content=VALUES(text_content),
  description=VALUES(description), is_active=1, updated_at=CURRENT_TIMESTAMP;
