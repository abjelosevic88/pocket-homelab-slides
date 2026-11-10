CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Slides ----------
-- A deck belongs to the person who made it (decks.user_id); nobody else on
-- the box sees it, box admins included. Other people see a deck only through
-- its links: the share link (read-only, when switched on), the audience link
-- (only while it is being presented, only the slide on the screen and the
-- ones before it, never the speaker notes) and a show's phone link (the
-- presenter screen, only while that show runs). Times are UTC ISO strings.

CREATE TABLE themes (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  base        TEXT NOT NULL DEFAULT 'paper',   -- the built-in theme it starts from
  bg          TEXT NOT NULL DEFAULT '',
  bg2         TEXT NOT NULL DEFAULT '',        -- second colour: a gentle gradient
  fg          TEXT NOT NULL DEFAULT '',
  muted       TEXT NOT NULL DEFAULT '',
  accent      TEXT NOT NULL DEFAULT '',
  head_font   TEXT NOT NULL DEFAULT '',
  body_font   TEXT NOT NULL DEFAULT '',
  logo_id     INTEGER,                          -- an upload shown in the corner
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX themes_user ON themes(user_id);

CREATE TABLE decks (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title          TEXT NOT NULL,
  about          TEXT NOT NULL DEFAULT '',
  theme          TEXT NOT NULL DEFAULT 'paper',  -- a built-in theme
  theme_id       INTEGER REFERENCES themes(id) ON DELETE SET NULL, -- or one of your own
  aspect         TEXT NOT NULL DEFAULT '16:9',   -- 16:9 | 4:3
  footer         TEXT NOT NULL DEFAULT '',       -- written at the foot of every slide
  numbers        INTEGER NOT NULL DEFAULT 1,     -- slide numbers in the corner
  transition     TEXT NOT NULL DEFAULT 'fade',   -- fade | slide | none
  target_minutes INTEGER NOT NULL DEFAULT 0,     -- how long the talk should take
  talk_title     TEXT NOT NULL DEFAULT '',       -- where and when it is given
  talk_at        TEXT NOT NULL DEFAULT '',       -- local YYYY-MM-DDTHH:MM
  talk_place     TEXT NOT NULL DEFAULT '',
  calendar_url   TEXT NOT NULL DEFAULT '',
  tags           TEXT NOT NULL DEFAULT '[]',
  template       INTEGER NOT NULL DEFAULT 0,     -- one of your own deck templates
  pinned         INTEGER NOT NULL DEFAULT 0,
  archived       INTEGER NOT NULL DEFAULT 0,
  shared         INTEGER NOT NULL DEFAULT 0,     -- the read-only share link works
  share_key      TEXT NOT NULL UNIQUE,
  live_key       TEXT NOT NULL UNIQUE,           -- the audience link (works while presenting)
  questions      INTEGER NOT NULL DEFAULT 1,     -- the audience may ask questions
  source         TEXT NOT NULL DEFAULT '',       -- the app that made it (Pocket Core link)
  source_ref     TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX decks_user ON decks(user_id, archived, updated_at);

CREATE TABLE uploads (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  deck_id    INTEGER REFERENCES decks(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  mime       TEXT NOT NULL DEFAULT '',
  data       BLOB NOT NULL,
  thumb      BLOB,
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX uploads_user ON uploads(user_id, deck_id);

CREATE TABLE slides (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  deck_id    INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL DEFAULT 0,
  layout     TEXT NOT NULL DEFAULT 'bullets',
  title      TEXT NOT NULL DEFAULT '',
  subtitle   TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',          -- short Markdown: - points, **bold**, *italic*, `code`
  body2      TEXT NOT NULL DEFAULT '',          -- the second column
  image_id   INTEGER REFERENCES uploads(id) ON DELETE SET NULL,
  data       TEXT NOT NULL DEFAULT '{}',        -- per layout: chart, table, poll options, code language, build
  notes      TEXT NOT NULL DEFAULT '',          -- speaker notes, never shown to the audience
  style      TEXT NOT NULL DEFAULT '',          -- '' | accent | dark | light
  hidden     INTEGER NOT NULL DEFAULT 0,        -- skipped when presenting
  seconds    INTEGER NOT NULL DEFAULT 0,        -- how long it should take
  source     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX slides_deck ON slides(deck_id, position);
CREATE INDEX slides_user ON slides(user_id);

-- My layouts: a slide kept to start new slides from, in any deck.
CREATE TABLE layouts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  layout     TEXT NOT NULL DEFAULT 'bullets',
  title      TEXT NOT NULL DEFAULT '',
  subtitle   TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  body2      TEXT NOT NULL DEFAULT '',
  data       TEXT NOT NULL DEFAULT '{}',
  notes      TEXT NOT NULL DEFAULT '',
  style      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX layouts_user ON layouts(user_id);

-- A show: one time a deck was presented (a talk) or practised (a rehearsal).
CREATE TABLE shows (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'talk',     -- talk | rehearsal
  remote_key  TEXT NOT NULL UNIQUE,             -- the phone link for this show
  pos         INTEGER NOT NULL DEFAULT 0,       -- the slide on the screen (among the shown ones)
  step        INTEGER NOT NULL DEFAULT 0,       -- points revealed so far on a step-by-step slide
  black       INTEGER NOT NULL DEFAULT 0,
  started_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  ended_at    TEXT,
  timings     TEXT NOT NULL DEFAULT '{}',       -- milliseconds spent per slide id
  total_ms    INTEGER NOT NULL DEFAULT 0,
  viewers     INTEGER NOT NULL DEFAULT 0,       -- most people following along at once
  reactions   INTEGER NOT NULL DEFAULT 0,
  note        TEXT NOT NULL DEFAULT ''          -- how it went
);
CREATE INDEX shows_user ON shows(user_id, deck_id, started_at);

CREATE TABLE votes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  show_id    INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- the deck's maker
  slide_id   INTEGER NOT NULL,
  choice     INTEGER NOT NULL,
  voter      TEXT NOT NULL,                     -- a random id kept by the voter's browser
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (show_id, slide_id, voter)
);
CREATE INDEX votes_user ON votes(user_id, show_id);

CREATE TABLE questions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  show_id    INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- the deck's maker
  slide_id   INTEGER,
  name       TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL,
  votes      INTEGER NOT NULL DEFAULT 0,
  answered   INTEGER NOT NULL DEFAULT 0,
  hidden     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX questions_user ON questions(user_id, show_id);

-- What the audience thought, after a talk.
CREATE TABLE feedback (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  show_id    INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  rating     INTEGER NOT NULL DEFAULT 0,        -- 1..5
  comment    TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',
  voter      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (show_id, voter)
);
CREATE INDEX feedback_user ON feedback(user_id, show_id);
