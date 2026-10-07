-- READ ONLY. Run against the deployment database and return results; omit secrets/PII.
SELECT DATABASE() AS db_name, VERSION() AS mysql_version, @@session.time_zone AS session_timezone, @@global.time_zone AS global_timezone;
SELECT table_name, column_name, column_type, is_nullable, column_default
FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN
('auto_urge_sms_state','admin_sms_send_record','wx_subscribe_delivery','welcome_email_delivery','project','project_application','project_members','project_event')
ORDER BY table_name,ordinal_position;
SELECT table_name,index_name,non_unique,seq_in_index,column_name
FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN
('auto_urge_sms_state','admin_sms_send_record','wx_subscribe_delivery','welcome_email_delivery','project','project_application','project_members','project_event','olive_branch_record')
ORDER BY table_name,index_name,seq_in_index;
SELECT table_name,constraint_name,column_name,referenced_table_name,referenced_column_name
FROM information_schema.key_column_usage WHERE constraint_schema=DATABASE() AND referenced_table_name IS NOT NULL;
SELECT id,project_id,event_id FROM project_event pe WHERE NOT EXISTS(SELECT 1 FROM project p WHERE p.id=pe.project_id) OR NOT EXISTS(SELECT 1 FROM event e WHERE e.id=pe.event_id);
SELECT id,project_id,order_id,status,channel,business_tag,max_recipients,total_sent,created_at,completed_at
FROM email_promotion WHERE status IN (0,1) ORDER BY id;
SELECT p.id AS promotion_id,p.order_id,p.status AS promotion_status,o.status AS order_status,o.push_status,o.refund_status,
 (SELECT COUNT(*) FROM email_task t WHERE t.promotion_id=p.id AND t.status=2) AS successful_tasks,
 (SELECT COUNT(*) FROM email_task t WHERE t.promotion_id=p.id AND t.status=3) AS failed_tasks
FROM email_promotion p LEFT JOIN `order` o ON o.id=p.order_id WHERE p.status IN (0,1) ORDER BY p.id;
SELECT state,COUNT(*) AS records FROM auto_urge_sms_state GROUP BY state;
SELECT template_key,status,COUNT(*) AS records FROM admin_sms_send_record GROUP BY template_key,status;
SELECT status,COUNT(*) AS records FROM wx_subscribe_delivery GROUP BY status;
SELECT status,COUNT(*) AS records FROM welcome_email_delivery GROUP BY status;
-- No UPDATE/DELETE, no resending and no refunds are performed by this file.
