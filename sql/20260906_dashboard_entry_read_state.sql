-- Review and run before deploying the backend and then the Mini Program.
-- The supplied export contains INSERTs only; inspect SHOW CREATE TABLE first.
-- Preserve the existing composite unique key (user_id,target_type,target_id,interaction_type).
-- Allow the independent entry cursor alongside the four legacy interaction kinds.
ALTER TABLE interaction_dashboard_view_state
  MODIFY COLUMN interaction_type VARCHAR(16) NOT NULL;

-- Existing users have already entered the dashboard when they read any detail tab.
-- Initialize the entry cursor from their latest detail read, without modifying detail cursors.
-- Re-running this backfill never advances an existing entry cursor.
INSERT INTO interaction_dashboard_view_state
  (user_id, target_type, target_id, interaction_type, last_viewed_at)
SELECT user_id, target_type, target_id, 'entry', MAX(last_viewed_at)
FROM interaction_dashboard_view_state
WHERE interaction_type IN ('like', 'favorite', 'share', 'visit')
GROUP BY user_id, target_type, target_id
ON DUPLICATE KEY UPDATE interaction_type = 'entry';
