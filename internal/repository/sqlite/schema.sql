CREATE TABLE IF NOT EXISTS banned_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip TEXT, name TEXT, hash TEXT, reason TEXT, created_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS executed_commands (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip TEXT, command_name TEXT, arguments TEXT, status TEXT,
  created_on INTEGER NOT NULL, channel TEXT
);
CREATE TABLE IF NOT EXISTS mail (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner TEXT NOT NULL, receiver TEXT NOT NULL, message TEXT, status TEXT NOT NULL,
  created_on INTEGER NOT NULL, is_whisper TEXT,
  text_encoding TEXT NOT NULL DEFAULT 'JSON_STRING'
);
CREATE TABLE IF NOT EXISTS mail_delivery (
  mail_id INTEGER NOT NULL REFERENCES mail(id),
  recipient_trip TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('PENDING', 'ATTEMPTING', 'ACCEPTED', 'UNKNOWN')),
  updated_on INTEGER NOT NULL,
  PRIMARY KEY (mail_id, recipient_trip)
);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip TEXT, name TEXT NOT NULL, hash TEXT, message TEXT,
  created_on INTEGER NOT NULL, channel TEXT,
  visibility TEXT DEFAULT 'PUBLIC' CHECK (visibility IN ('PUBLIC', 'WHISPER'))
);
CREATE TABLE IF NOT EXISTS user_presence_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip TEXT, name TEXT, hash TEXT, event_type TEXT,
  created_on INTEGER NOT NULL, channel TEXT
);
CREATE TABLE IF NOT EXISTS notes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip TEXT, note TEXT, created_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS trips (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL CONSTRAINT trips_type_check CHECK (type IN ('ADMIN', 'MODERATOR', 'TRUSTED', 'USER', 'REGULAR', 'PEST')),
  trip TEXT UNIQUE, created_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS names (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT UNIQUE, created_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS trip_names (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trip_id INTEGER NOT NULL, name_id INTEGER NOT NULL,
  FOREIGN KEY (trip_id) REFERENCES trips(id),
  FOREIGN KEY (name_id) REFERENCES names(id),
  UNIQUE (trip_id, name_id)
);
CREATE TABLE IF NOT EXISTS dbz_characters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT, level INTEGER, created_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS dbz_stats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  char_id INTEGER NOT NULL, str INTEGER, agi INTEGER, vit INTEGER, ene INTEGER,
  free_stats INTEGER, created_on INTEGER NOT NULL,
  FOREIGN KEY (char_id) REFERENCES dbz_characters(id)
);
CREATE TABLE IF NOT EXISTS agent_memory (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key TEXT NOT NULL, role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  content TEXT NOT NULL, created_on INTEGER NOT NULL, expires_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_tool_memory (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  identity_key TEXT NOT NULL, tool_name TEXT NOT NULL, content TEXT NOT NULL,
  created_on INTEGER NOT NULL, expires_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_memory_summary (
  identity_key TEXT PRIMARY KEY NOT NULL, content TEXT NOT NULL,
  covered_through_id INTEGER NOT NULL, fingerprint TEXT NOT NULL,
  created_on INTEGER NOT NULL, expires_on INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_messages_trip_created_on ON messages (trip, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_messages_name_created_on ON messages (name, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_messages_hash_created_on ON messages (hash, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_messages_channel_created_on ON messages (channel, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_agent_messages_name_room_visibility_created ON messages (name, channel, visibility, created_on DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_agent_messages_name_visibility_created ON messages (name, visibility, created_on DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_agent_messages_room_visibility_created ON messages (channel, visibility, created_on DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_agent_messages_trip_visibility_created ON messages (trip, visibility, created_on DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_agent_messages_visibility ON messages (visibility);
CREATE INDEX IF NOT EXISTS idx_mail_status_receiver ON mail (status, receiver);
CREATE INDEX IF NOT EXISTS idx_notes_trip_created_on ON notes (trip, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_executed_commands_channel_created_on ON executed_commands (channel, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_banned_users_name ON banned_users (name);
CREATE INDEX IF NOT EXISTS idx_banned_users_trip ON banned_users (trip);
CREATE INDEX IF NOT EXISTS idx_banned_users_hash ON banned_users (hash);
CREATE INDEX IF NOT EXISTS idx_presence_identity_created ON user_presence_log (trip, hash, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_agent_memory_identity_created ON agent_memory (identity_key, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_agent_memory_expires ON agent_memory (expires_on);
CREATE INDEX IF NOT EXISTS idx_agent_tool_memory_identity_created ON agent_tool_memory (identity_key, created_on DESC);
CREATE INDEX IF NOT EXISTS idx_agent_memory_summary_expires ON agent_memory_summary (expires_on);
