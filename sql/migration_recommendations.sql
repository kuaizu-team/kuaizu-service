-- Info center recommendation tables.
-- Safe to run after event/project migrations; existing information_content data is untouched.

CREATE TABLE IF NOT EXISTS project_recommendation (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id INT NOT NULL,
  display_order INT NOT NULL DEFAULT 0 COMMENT '展示权重',
  is_visible TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否显示',
  is_featured TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否推荐到首页',
  interview_url VARCHAR(500) NULL COMMENT '采访链接',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_project_recommendation_project (project_id),
  KEY idx_project_recommendation_visible_order (is_visible, display_order, created_at),
  KEY idx_project_recommendation_featured (is_featured, is_visible, display_order, created_at),
  CONSTRAINT fk_project_recommendation_project FOREIGN KEY (project_id) REFERENCES project(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='校园项目推荐';

CREATE TABLE IF NOT EXISTS podcast_recommendation (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  title VARCHAR(200) NOT NULL,
  description TEXT NULL,
  article_url VARCHAR(500) NOT NULL,
  display_order INT NOT NULL DEFAULT 0 COMMENT '展示权重',
  is_visible TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否显示',
  is_featured TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否推荐到首页',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_podcast_recommendation_visible_order (is_visible, display_order, created_at),
  KEY idx_podcast_recommendation_featured (is_featured, is_visible, display_order, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='快组Talking播客推荐';

CREATE TABLE IF NOT EXISTS news_recommendation (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  title VARCHAR(200) NOT NULL,
  description TEXT NULL,
  article_url VARCHAR(500) NOT NULL,
  display_order INT NOT NULL DEFAULT 0 COMMENT '展示权重',
  is_visible TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否显示',
  is_featured TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否推荐到首页',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_news_recommendation_visible_order (is_visible, display_order, created_at),
  KEY idx_news_recommendation_featured (is_featured, is_visible, display_order, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='我们的资讯推荐';
