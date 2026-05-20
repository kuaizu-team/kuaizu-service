-- Add is_read field to olive_branch_record
-- Meaning: whether the receiver has viewed this olive branch invitation
ALTER TABLE olive_branch_record ADD COLUMN is_read BOOLEAN NOT NULL DEFAULT FALSE;

-- Add is_read field to project_application
-- Meaning: whether the project owner (reviewer) has viewed this application
ALTER TABLE project_application ADD COLUMN is_read BOOLEAN NOT NULL DEFAULT FALSE;
