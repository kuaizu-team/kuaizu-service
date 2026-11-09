-- MySQL 8.0.13+. Manual migration; NOT executed by Codex.
-- No table/route additions. Only add links[].public where absent/null.
-- Legacy links_visible=1 -> public=true; =0 -> public=false (approval-only).
-- Explicit per-link flags, link order, applications, contacts and audit timestamps are preserved.
-- Back up campus_cooperation first, select the intended database, then run this whole script.
-- Run before releasing the new backend/admin together; refresh any open admin editors.
-- Repeating this script keeps already configured per-link flags unchanged.

DROP PROCEDURE IF EXISTS migrate_cooperation_link_public_20261026;
DELIMITER $$
CREATE PROCEDURE migrate_cooperation_link_public_20261026()
BEGIN
  DECLARE finished BOOLEAN DEFAULT FALSE;
  DECLARE cooperation_id BIGINT;
  DECLARE legacy_public BOOLEAN;
  DECLARE old_links JSON;
  DECLARE new_links JSON;
  DECLARE link_index INT;
  DECLARE public_path VARCHAR(80);
  DECLARE rows_to_migrate CURSOR FOR
    SELECT id, links_visible, links
    FROM campus_cooperation
    WHERE JSON_TYPE(links) = 'ARRAY' AND JSON_LENGTH(links) > 0
    ORDER BY id FOR UPDATE;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET finished = TRUE;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION
  BEGIN
    ROLLBACK;
    RESIGNAL;
  END;

  IF DATABASE() IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Select the intended database first';
  END IF;

  START TRANSACTION;
  OPEN rows_to_migrate;
  migrate_rows: LOOP
    FETCH rows_to_migrate INTO cooperation_id, legacy_public, old_links;
    IF finished THEN LEAVE migrate_rows; END IF;
    SET new_links = old_links;
    SET link_index = 0;
    WHILE link_index < JSON_LENGTH(new_links) DO
      SET public_path = CONCAT('$[', link_index, '].public');
      IF JSON_CONTAINS_PATH(new_links, 'one', public_path) = 0
         OR JSON_TYPE(JSON_EXTRACT(new_links, public_path)) = 'NULL' THEN
        SET new_links = JSON_SET(new_links, public_path,
          JSON_EXTRACT(IF(legacy_public, 'true', 'false'), '$'));
      END IF;
      SET link_index = link_index + 1;
    END WHILE;
    IF NOT (CAST(old_links AS CHAR) <=> CAST(new_links AS CHAR)) THEN
      UPDATE campus_cooperation
      SET links = new_links, updated_at = updated_at
      WHERE id = cooperation_id;
    END IF;
  END LOOP;
  CLOSE rows_to_migrate;
  COMMIT;
END$$
DELIMITER ;
CALL migrate_cooperation_link_public_20261026();
DROP PROCEDURE migrate_cooperation_link_public_20261026;

-- Verify: every row must have public_type=BOOLEAN.
-- CoCraft (current export id=2) should have public=false, application_required=1.
-- Its link is then visible only after that user's application is approved.
SELECT c.id, c.title, c.application_required, j.link_ordinal AS link_position,
       JSON_UNQUOTE(JSON_EXTRACT(c.links, CONCAT('$[', j.link_ordinal - 1, '].title'))) AS link_title,
       JSON_EXTRACT(c.links, CONCAT('$[', j.link_ordinal - 1, '].public')) AS is_public,
       JSON_TYPE(JSON_EXTRACT(c.links, CONCAT('$[', j.link_ordinal - 1, '].public'))) AS public_type
FROM campus_cooperation c
JOIN JSON_TABLE(c.links, '$[*]' COLUMNS (link_ordinal FOR ORDINALITY)) AS j
ORDER BY c.id, j.link_ordinal;
