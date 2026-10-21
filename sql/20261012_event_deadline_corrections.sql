-- 仅校正已有公开来源或已确认业务口径支持的报名截止日期。
-- 旧值条件可避免覆盖脚本执行前后的人工更新。
START TRANSACTION;

UPDATE `event`
SET registration_deadline = '2026-07-20', updated_at = CURRENT_TIMESTAMP
WHERE id = 43
  AND name = '中国软件杯大学生软件设计大赛'
  AND registration_deadline = '2027-06-30';

UPDATE `event`
SET registration_deadline = '2026-10-15', updated_at = CURRENT_TIMESTAMP
WHERE id = 81
  AND name = '全球校园人工智能算法精英大赛'
  AND registration_deadline = '2026-10-10';

-- 按已确认的本科/研究生赛道口径维护。
UPDATE `event`
SET registration_deadline = '2026-09-17', updated_at = CURRENT_TIMESTAMP
WHERE id = 144
  AND name = '大学生“AI＋信息素养”大赛'
  AND registration_deadline = '2026-09-22';

UPDATE `event`
SET registration_deadline = '2026-09-30', updated_at = CURRENT_TIMESTAMP
WHERE id = 152
  AND name = '全国大学生GIS应用技能大赛'
  AND registration_deadline = '2026-08-31';

COMMIT;

SELECT id, name, registration_deadline
FROM `event`
WHERE id IN (43, 81, 144, 152)
ORDER BY id;
