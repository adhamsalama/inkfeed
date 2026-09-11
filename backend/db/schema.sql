CREATE TABLE IF NOT EXISTS users (
    id            INTEGER  PRIMARY KEY AUTOINCREMENT,
    email         TEXT     NOT NULL UNIQUE,
    password_hash TEXT     NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT     PRIMARY KEY,
    user_id    INTEGER  NOT NULL REFERENCES users(id),
    expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id           INTEGER PRIMARY KEY REFERENCES users(id),
    font_size         REAL,
    letter_spacing    REAL,
    line_height       REAL,
    cors_proxy_url    TEXT,
    epub_embed_images INTEGER,
    mobi_embed_images INTEGER,
    email_to          TEXT,
    font_family       TEXT,
    bold_text         INTEGER,
    dark_mode         INTEGER,
    group_sort_by_date INTEGER NOT NULL DEFAULT 1,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_saved_feeds (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    url             TEXT    NOT NULL,
    title           TEXT    NOT NULL,
    position        INTEGER NOT NULL DEFAULT 0,
    archive_enabled INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_feed_groups (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id  INTEGER NOT NULL REFERENCES users(id),
    name     TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_feed_group_items (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES user_feed_groups(id),
    url      TEXT NOT NULL,
    title    TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_favorites (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER  NOT NULL REFERENCES users(id),
    url        TEXT     NOT NULL,
    title      TEXT     NOT NULL DEFAULT '',
    feed_title TEXT     NOT NULL DEFAULT '',
    pub_date     TEXT     NOT NULL DEFAULT '',
    comments_url TEXT     NOT NULL DEFAULT '',
    saved_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS article_archive (
    key          TEXT     PRIMARY KEY,
    title        TEXT     NOT NULL DEFAULT '',
    author       TEXT     NOT NULL DEFAULT '',
    site_name    TEXT     NOT NULL DEFAULT '',
    created_at   TEXT     NOT NULL DEFAULT '',
    html_content TEXT     NOT NULL DEFAULT '',
    text_content TEXT     NOT NULL DEFAULT '',
    archived_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ip_rate_limits (
    ip            TEXT     NOT NULL,
    endpoint      TEXT     NOT NULL,
    count         INTEGER  NOT NULL DEFAULT 0,
    window_start  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    blocked_until DATETIME,
    PRIMARY KEY (ip, endpoint)
);

CREATE TABLE IF NOT EXISTS feed_items (
    id             INTEGER  PRIMARY KEY AUTOINCREMENT,
    feed_url       TEXT     NOT NULL,
    item_url       TEXT     NOT NULL,
    title          TEXT     NOT NULL DEFAULT '',
    description    TEXT     NOT NULL DEFAULT '',
    pub_date       TEXT     NOT NULL DEFAULT '',
    scraped_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archive_failed INTEGER  NOT NULL DEFAULT 0,
    comments_url   TEXT,
    UNIQUE(feed_url, item_url)
);
