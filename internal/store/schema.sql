-- kaogong multi-user schema (SQLite)
-- Content tables are imported from the legacy tiku.db / materials.db.
-- User-owned tables always carry user_id. Never trust user_id from clients.

CREATE TABLE IF NOT EXISTS papers (
  id            INTEGER PRIMARY KEY,
  subject       TEXT NOT NULL,
  subjectName   TEXT NOT NULL,
  category      TEXT NOT NULL,
  name          TEXT NOT NULL,
  questionCount INTEGER,
  difficulty    REAL,
  chapters      TEXT
);
CREATE INDEX IF NOT EXISTS idx_papers_subject ON papers(subjectName, category);
CREATE INDEX IF NOT EXISTS idx_papers_name_year ON papers(name);

CREATE TABLE IF NOT EXISTS questions (
  id           INTEGER PRIMARY KEY,
  questionId   INTEGER NOT NULL,
  paperId      INTEGER NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
  chapter      TEXT,
  type         INTEGER,
  content      TEXT NOT NULL DEFAULT '',
  contentHtml  TEXT,
  options      TEXT,
  answer       TEXT,
  answerIndex  INTEGER,
  difficulty   INTEGER,
  analysis     TEXT
);
CREATE INDEX IF NOT EXISTS idx_questions_qid ON questions(questionId);
CREATE INDEX IF NOT EXISTS idx_questions_paper ON questions(paperId, id);
CREATE INDEX IF NOT EXISTS idx_questions_chapter ON questions(chapter);

CREATE TABLE IF NOT EXISTS question_categories (
  question_id INTEGER NOT NULL,
  subject     TEXT NOT NULL,
  category    TEXT NOT NULL,
  sub         TEXT NOT NULL,
  PRIMARY KEY (question_id, subject)
);
CREATE INDEX IF NOT EXISTS idx_qc_category ON question_categories(subject, category, sub);

CREATE TABLE IF NOT EXISTS q_materials (
  material_id INTEGER NOT NULL,
  subject     TEXT NOT NULL,
  content     TEXT,
  PRIMARY KEY (material_id, subject)
);

CREATE TABLE IF NOT EXISTS q_material_map (
  question_id INTEGER NOT NULL,
  subject     TEXT NOT NULL,
  material_id INTEGER,
  PRIMARY KEY (question_id, subject)
);

CREATE TABLE IF NOT EXISTS materials (
  paperId INTEGER NOT NULL,
  title   TEXT NOT NULL DEFAULT '',
  idx     INTEGER NOT NULL DEFAULT 0,
  text    TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (paperId, title)
);
CREATE INDEX IF NOT EXISTS idx_materials_paper ON materials(paperId, idx);

CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
  status        INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  last_login_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  expires_at INTEGER NOT NULL,
  user_agent TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT '',
  revoked_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);

-- MCP access tokens are bearer credentials for external agents. Only the
-- SHA-256 hash is stored; the plaintext token is printed once at creation.
CREATE TABLE IF NOT EXISTS mcp_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  scope        TEXT NOT NULL DEFAULT 'read',
  created_at   TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  expires_at   INTEGER NOT NULL,
  last_used_at TEXT,
  revoked_at   INTEGER
);
CREATE INDEX IF NOT EXISTS idx_mcp_tokens_user ON mcp_tokens(user_id);

CREATE TABLE IF NOT EXISTS practice_records (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question_id   TEXT NOT NULL,
  paper_id      INTEGER,
  subject       TEXT NOT NULL DEFAULT '',
  chapter       TEXT NOT NULL DEFAULT '',
  question_type INTEGER NOT NULL DEFAULT 0,
  selected      TEXT NOT NULL DEFAULT '',
  is_correct    INTEGER,
  cost_ms       INTEGER NOT NULL DEFAULT 0,
  archived      INTEGER NOT NULL DEFAULT 0,
  group_key     TEXT NOT NULL DEFAULT '',
  sub_key       TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_records_user_time ON practice_records(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_records_user_qid ON practice_records(user_id, question_id);
CREATE INDEX IF NOT EXISTS idx_records_user_wrong ON practice_records(user_id, is_correct, archived);
CREATE INDEX IF NOT EXISTS idx_records_group ON practice_records(user_id, group_key, sub_key, archived, is_correct);

CREATE TABLE IF NOT EXISTS favorites (
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question_id TEXT NOT NULL,
  subject     TEXT NOT NULL DEFAULT '',
  chapter     TEXT NOT NULL DEFAULT '',
  group_key   TEXT NOT NULL DEFAULT '',
  sub_key     TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  PRIMARY KEY (user_id, question_id)
);
CREATE INDEX IF NOT EXISTS idx_favorites_user_time ON favorites(user_id, created_at);

CREATE TABLE IF NOT EXISTS notes (
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question_id TEXT NOT NULL,
  subject     TEXT NOT NULL DEFAULT '',
  chapter     TEXT NOT NULL DEFAULT '',
  note        TEXT NOT NULL DEFAULT '',
  group_key   TEXT NOT NULL DEFAULT '',
  sub_key     TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  updated_at  TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  PRIMARY KEY (user_id, question_id)
);
CREATE INDEX IF NOT EXISTS idx_notes_user_time ON notes(user_id, updated_at);

CREATE TABLE IF NOT EXISTS custom_batches (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  subject    TEXT NOT NULL DEFAULT '自定义',
  created_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_custom_batches_user ON custom_batches(user_id);

CREATE TABLE IF NOT EXISTS custom_questions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id     INTEGER NOT NULL REFERENCES custom_batches(id) ON DELETE CASCADE,
  prompt       TEXT NOT NULL DEFAULT '',
  material     TEXT NOT NULL DEFAULT '',
  options      TEXT NOT NULL DEFAULT '[]',
  answer       TEXT NOT NULL DEFAULT '',
  answer_index INTEGER NOT NULL DEFAULT -1,
  analysis     TEXT NOT NULL DEFAULT '',
  images       TEXT NOT NULL DEFAULT '[]',
  material_id  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_custom_questions_batch ON custom_questions(batch_id);

CREATE TABLE IF NOT EXISTS ai_agents (
  id               INTEGER PRIMARY KEY,
  name             TEXT NOT NULL,
  role             TEXT NOT NULL UNIQUE,
  description      TEXT NOT NULL DEFAULT '',
  system_prompt    TEXT NOT NULL DEFAULT '',
  skill            TEXT NOT NULL DEFAULT '',
  base_url         TEXT NOT NULL DEFAULT '',
  api_key_enc      TEXT NOT NULL DEFAULT '',
  model            TEXT NOT NULL DEFAULT '',
  temperature      REAL NOT NULL DEFAULT 0.5,
  max_tokens       INTEGER NOT NULL DEFAULT 1500,
  enabled          INTEGER NOT NULL DEFAULT 0,
  reasoning_effort TEXT NOT NULL DEFAULT 'low',
  updated_at       TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);

CREATE TABLE IF NOT EXISTS prompt_history (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id      INTEGER NOT NULL REFERENCES ai_agents(id) ON DELETE CASCADE,
  system_prompt TEXT NOT NULL DEFAULT '',
  skill         TEXT NOT NULL DEFAULT '',
  note          TEXT NOT NULL DEFAULT '',
  saved_at      TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);

CREATE TABLE IF NOT EXISTS user_skills (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  text        TEXT NOT NULL DEFAULT '',
  files       TEXT NOT NULL DEFAULT '[]',
  created_at  TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  UNIQUE (user_id, name)
);

CREATE TABLE IF NOT EXISTS ai_explains (
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question_id TEXT NOT NULL,
  selected    TEXT NOT NULL DEFAULT 'none',
  correct     TEXT NOT NULL DEFAULT 'none',
  content     TEXT NOT NULL DEFAULT '',
  image_note  TEXT NOT NULL DEFAULT '',
  model       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (datetime('now', 'localtime')),
  PRIMARY KEY (user_id, question_id, selected, correct)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_id   INTEGER,
  action     TEXT NOT NULL,
  target     TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT '',
  detail     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor_id);

PRAGMA user_version = 1;
