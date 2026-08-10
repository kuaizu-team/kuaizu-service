-- Read-only schema preflight for feat--api--website-public-team-api.
-- PASS: this script returns zero rows. Any returned row names a missing object.

SELECT 'event.level' AS missing_object
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'event' AND column_name = 'level')
UNION ALL SELECT 'event.summary'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'event' AND column_name = 'summary')
UNION ALL SELECT 'event.school_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'event' AND column_name = 'school_id')
UNION ALL SELECT 'event.admin_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'event' AND column_name = 'admin_id')
UNION ALL SELECT 'event.creator_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'event' AND column_name = 'creator_id')
UNION ALL SELECT 'admin_user.join_date'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'admin_user' AND column_name = 'join_date')
UNION ALL SELECT 'admin_user.intro'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'admin_user' AND column_name = 'intro')
UNION ALL SELECT 'admin_user.article_url'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'admin_user' AND column_name = 'article_url')
UNION ALL SELECT 'admin_school_relation table'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'admin_school_relation')
UNION ALL SELECT 'welcome_email_delivery table'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'welcome_email_delivery')
UNION ALL SELECT 'settlement_record.beneficiary_admin_user_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'settlement_record' AND column_name = 'beneficiary_admin_user_id')
UNION ALL SELECT 'settlement_record.commission_rate'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'settlement_record' AND column_name = 'commission_rate')
UNION ALL SELECT 'settlement_record_order.beneficiary_admin_user_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'settlement_record_order' AND column_name = 'beneficiary_admin_user_id')
UNION ALL SELECT 'order.push_status'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'push_status')
UNION ALL SELECT 'order.push_retry_count'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'push_retry_count')
UNION ALL SELECT 'order.last_push_time'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'last_push_time')
UNION ALL SELECT 'order.push_error_message'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'push_error_message')
UNION ALL SELECT 'event.uk_event_admin_id'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'event' AND constraint_name = 'uk_event_admin_id' AND constraint_type = 'UNIQUE')
UNION ALL SELECT 'event.fk_event_admin'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'event' AND constraint_name = 'fk_event_admin' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'event.fk_event_creator'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'event' AND constraint_name = 'fk_event_creator' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'admin_school_relation.uk_admin_school'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'admin_school_relation' AND constraint_name = 'uk_admin_school' AND constraint_type = 'UNIQUE')
UNION ALL SELECT 'admin_school_relation.uk_school_owner'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'admin_school_relation' AND constraint_name = 'uk_school_owner' AND constraint_type = 'UNIQUE')
UNION ALL SELECT 'admin_school_relation.fk_admin_school_admin'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'admin_school_relation' AND constraint_name = 'fk_admin_school_admin' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'admin_school_relation.fk_admin_school_school'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'admin_school_relation' AND constraint_name = 'fk_admin_school_school' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'welcome_email_delivery.fk_welcome_email_user'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'welcome_email_delivery' AND constraint_name = 'fk_welcome_email_user' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'welcome_email_delivery.idx_welcome_email_status'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'welcome_email_delivery' AND index_name = 'idx_welcome_email_status')
UNION ALL SELECT 'settlement_record.fk_settlement_beneficiary'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'settlement_record' AND constraint_name = 'fk_settlement_beneficiary' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'settlement_record_order.fk_settlement_order_beneficiary'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'settlement_record_order' AND constraint_name = 'fk_settlement_order_beneficiary' AND constraint_type = 'FOREIGN KEY')
UNION ALL SELECT 'settlement_record_order.uk_order_beneficiary'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'settlement_record_order' AND constraint_name = 'uk_order_beneficiary' AND constraint_type = 'UNIQUE')
UNION ALL SELECT 'trg_admin_school_rate_before_insert'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'trg_admin_school_rate_before_insert')
UNION ALL SELECT 'trg_admin_school_rate_before_update'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'trg_admin_school_rate_before_update');
