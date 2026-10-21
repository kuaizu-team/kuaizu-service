-- 已确认的赛事重复记录清理。可重复执行。
-- 保留：43（中国软件杯）、151（校级建行杯）。
-- 删除：2、142、150。
START TRANSACTION;

-- 将 ID 2 中更完整的公开信息补到榜单主记录 ID 43。
UPDATE `event` target
LEFT JOIN `event` source ON source.id = 2
SET target.name = '中国软件杯大学生软件设计大赛',
    target.is_ranking = 1,
    target.article_url = COALESCE(NULLIF(target.article_url, ''), source.article_url),
    target.summary = COALESCE(target.summary, source.summary),
    target.organizer_name = COALESCE(target.organizer_name, source.organizer_name),
    target.description = COALESCE(target.description, source.description),
    target.resource_url = COALESCE(target.resource_url, source.resource_url),
    target.official_website = COALESCE(target.official_website, source.official_website),
    target.updated_at = CURRENT_TIMESTAMP
WHERE target.id = 43
  AND target.name IN ('"中国软件杯"大学生软件设计大赛', '中国软件杯大学生软件设计大赛')
  AND (
    source.id IS NOT NULL
    OR target.name <> '中国软件杯大学生软件设计大赛'
    OR target.is_ranking <> 1
  );

INSERT IGNORE INTO information_event (information_id, event_id, created_at)
SELECT information_id, 43, created_at
FROM information_event
WHERE event_id = 2
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 2 AND source.name = '中国软件杯大学生软件设计大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 43);

INSERT IGNORE INTO project_event (project_id, event_id, created_at)
SELECT project_id, 43, created_at
FROM project_event
WHERE event_id = 2
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 2 AND source.name = '中国软件杯大学生软件设计大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 43);

-- ID 43 的 2027 时间线无已公布届次依据；仅在 ID 2 仍存在时替换为其 2026 时间线。
DELETE node
FROM event_timeline_node node
WHERE node.event_id = 43
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 2 AND source.name = '中国软件杯大学生软件设计大赛'
  );

UPDATE event_timeline_node
SET event_id = 43
WHERE event_id = 2
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 2 AND source.name = '中国软件杯大学生软件设计大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 43);

DELETE relation
FROM information_event relation
JOIN `event` source ON source.id = relation.event_id
WHERE source.id = 2
  AND source.name = '中国软件杯大学生软件设计大赛';
DELETE relation
FROM project_event relation
JOIN `event` source ON source.id = relation.event_id
WHERE source.id = 2
  AND source.name = '中国软件杯大学生软件设计大赛';
DELETE source
FROM `event` source
JOIN `event` target ON target.id = 43
WHERE source.id = 2
  AND source.name = '中国软件杯大学生软件设计大赛';

-- ID 150 与 151 的业务字段和时间线相同，保留有截止日期的 ID 151。
INSERT IGNORE INTO information_event (information_id, event_id, created_at)
SELECT information_id, 151, created_at
FROM information_event
WHERE event_id = 150
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 150 AND source.name = '“建行杯”中国国际大学生创新大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 151);

INSERT IGNORE INTO project_event (project_id, event_id, created_at)
SELECT project_id, 151, created_at
FROM project_event
WHERE event_id = 150
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 150 AND source.name = '“建行杯”中国国际大学生创新大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 151);

DELETE node
FROM event_timeline_node node
WHERE node.event_id = 150
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 150 AND source.name = '“建行杯”中国国际大学生创新大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` WHERE id = 151);

DELETE FROM information_event
WHERE event_id = 150
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 150 AND source.name = '“建行杯”中国国际大学生创新大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` target WHERE target.id = 151);
DELETE FROM project_event
WHERE event_id = 150
  AND EXISTS (
    SELECT 1 FROM `event` source
    WHERE source.id = 150 AND source.name = '“建行杯”中国国际大学生创新大赛'
  )
  AND EXISTS (SELECT 1 FROM `event` target WHERE target.id = 151);
DELETE source
FROM `event` source
JOIN `event` target ON target.id = 151
WHERE source.id = 150
  AND source.name = '“建行杯”中国国际大学生创新大赛'
  AND target.name = '“建行杯”中国国际大学生创新大赛';

-- ID 142 是无描述、无来源、无业务含义的“比赛”记录。
DELETE relation
FROM event_timeline_node relation
JOIN `event` target ON target.id = relation.event_id
WHERE target.id = 142 AND target.name = '比赛';
DELETE relation
FROM information_event relation
JOIN `event` target ON target.id = relation.event_id
WHERE target.id = 142 AND target.name = '比赛';
DELETE relation
FROM project_event relation
JOIN `event` target ON target.id = relation.event_id
WHERE target.id = 142 AND target.name = '比赛';
DELETE FROM `event` WHERE id = 142 AND name = '比赛';

COMMIT;

SELECT id, name, registration_deadline
FROM `event`
WHERE id IN (2, 43, 142, 150, 151)
ORDER BY id;
