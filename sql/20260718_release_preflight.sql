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
UNION ALL SELECT 'admin_user.password_encrypted'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'admin_user' AND column_name = 'password_encrypted')
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
UNION ALL SELECT 'admin_school_relation.uk_school_owner'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'admin_school_relation' AND index_name = 'uk_school_owner')
UNION ALL SELECT 'trg_admin_school_rate_before_insert'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'trg_admin_school_rate_before_insert')
UNION ALL SELECT 'trg_admin_school_rate_before_update'
WHERE NOT EXISTS (SELECT 1 FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'trg_admin_school_rate_before_update');