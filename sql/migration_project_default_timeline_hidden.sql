-- Apply before deploying the backend. Does not modify created_at or project_milestones.
ALTER TABLE project
  ADD COLUMN default_timeline_hidden TINYINT(1) NOT NULL DEFAULT 0
  COMMENT 'Whether the derived project-created timeline node is hidden';
