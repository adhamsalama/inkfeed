-- name: CreateUser :one
INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?);

-- name: GetSession :one
SELECT * FROM sessions WHERE token = ? AND expires_at > CURRENT_TIMESTAMP LIMIT 1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ? LIMIT 1;

-- name: GetUserPreferences :one
SELECT font_size, letter_spacing, line_height, cors_proxy_url, epub_embed_images, mobi_embed_images, email_to, font_family, bold_text, dark_mode, group_sort_by_date
FROM user_preferences WHERE user_id = ? LIMIT 1;

-- name: UpsertUserPreferences :exec
INSERT INTO user_preferences (user_id, font_size, letter_spacing, line_height, cors_proxy_url, epub_embed_images, mobi_embed_images, email_to, font_family, bold_text, dark_mode, group_sort_by_date, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(user_id) DO UPDATE SET
    font_size = excluded.font_size,
    letter_spacing = excluded.letter_spacing,
    line_height = excluded.line_height,
    cors_proxy_url = excluded.cors_proxy_url,
    epub_embed_images = excluded.epub_embed_images,
    mobi_embed_images = excluded.mobi_embed_images,
    email_to = excluded.email_to,
    font_family = excluded.font_family,
    bold_text = excluded.bold_text,
    dark_mode = excluded.dark_mode,
    group_sort_by_date = excluded.group_sort_by_date,
    updated_at = CURRENT_TIMESTAMP;

-- name: GetUserSavedFeeds :many
SELECT url, title, archive_enabled FROM user_saved_feeds WHERE user_id = ? ORDER BY position;

-- name: DeleteUserSavedFeeds :exec
DELETE FROM user_saved_feeds WHERE user_id = ?;

-- name: InsertUserSavedFeed :exec
INSERT INTO user_saved_feeds (user_id, url, title, position, archive_enabled) VALUES (?, ?, ?, ?, ?);

-- name: GetUserFeedGroups :many
SELECT id, name FROM user_feed_groups WHERE user_id = ? ORDER BY position;

-- name: GetFeedGroupItems :many
SELECT url, title FROM user_feed_group_items WHERE group_id = ? ORDER BY position;

-- name: DeleteUserFeedGroupItems :exec
DELETE FROM user_feed_group_items WHERE group_id IN (SELECT id FROM user_feed_groups WHERE user_id = ?);

-- name: DeleteUserFeedGroups :exec
DELETE FROM user_feed_groups WHERE user_id = ?;

-- name: InsertFeedGroup :one
INSERT INTO user_feed_groups (user_id, name, position) VALUES (?, ?, ?) RETURNING id;

-- name: InsertFeedGroupItem :exec
INSERT INTO user_feed_group_items (group_id, url, title, position) VALUES (?, ?, ?, ?);


-- name: GetUserFavorites :many
SELECT url, title, feed_title, pub_date, comments_url FROM user_favorites WHERE user_id = ? ORDER BY saved_at DESC;

-- name: DeleteAllUserFavorites :exec
DELETE FROM user_favorites WHERE user_id = ?;

-- name: InsertUserFavorite :exec
INSERT INTO user_favorites (user_id, url, title, feed_title, pub_date, comments_url) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetArticleArchive :one
SELECT title, author, site_name, created_at, html_content, text_content FROM article_archive WHERE key = ? LIMIT 1;


-- name: InsertFeedItem :execresult
INSERT OR IGNORE INTO feed_items (feed_url, item_url, title, description, pub_date, comments_url)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetDistinctSavedFeedURLs :many
SELECT DISTINCT url FROM user_saved_feeds WHERE archive_enabled = 1;

-- name: GetNextFeedItemWithoutArchive :one
SELECT item_url FROM feed_items
WHERE item_url NOT IN (SELECT key FROM article_archive)
AND archive_failed = 0
LIMIT 1;

-- name: MarkFeedItemArchiveFailed :exec
UPDATE feed_items SET archive_failed = 1 WHERE item_url = ?;

-- name: GetFeedArchiveItems :many
SELECT item_url, title, description, pub_date, scraped_at, comments_url
FROM feed_items
WHERE feed_url = ?
ORDER BY datetime(pub_date) DESC, scraped_at DESC
LIMIT ? OFFSET ?;

-- name: CountFeedArchiveItems :one
SELECT COUNT(*) FROM feed_items WHERE feed_url = ?;

-- name: DeleteOldFeedItems :execresult
DELETE FROM feed_items WHERE scraped_at < datetime('now', '-' || ? || ' hours');


-- name: UpsertArticleArchive :exec
INSERT INTO article_archive (key, title, author, site_name, created_at, html_content, text_content) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET title = excluded.title, author = excluded.author, site_name = excluded.site_name, created_at = excluded.created_at, html_content = excluded.html_content, text_content = excluded.text_content, updated_at = CURRENT_TIMESTAMP;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE id = ?;

-- name: GetIPRateLimit :one
SELECT count, window_start, blocked_until FROM ip_rate_limits WHERE ip = ? AND endpoint = ? LIMIT 1;

-- name: UpsertIPRateLimit :exec
INSERT INTO ip_rate_limits (ip, endpoint, count, window_start, blocked_until)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(ip, endpoint) DO UPDATE SET
    count = excluded.count,
    window_start = excluded.window_start,
    blocked_until = excluded.blocked_until;

-- name: GetArticleArchiveTotalSize :one
SELECT CAST(COALESCE(SUM(LENGTH(html_content) + LENGTH(text_content)), 0) AS INTEGER) AS total_size FROM article_archive;

-- name: GetOldestArticleArchiveKey :one
SELECT key, title FROM article_archive ORDER BY archived_at ASC LIMIT 1;

-- name: DeleteOldestArticleArchiveRow :exec
DELETE FROM article_archive
WHERE key = (SELECT key FROM article_archive ORDER BY archived_at ASC LIMIT 1);

-- name: UserHasSavedFeed :one
SELECT EXISTS (SELECT 1 FROM user_saved_feeds WHERE user_id = ? AND url = ?);

-- name: SearchFeedArchive :many
SELECT fi.item_url, fi.title, fi.description, fi.pub_date, fi.comments_url, a.text_content
FROM article_fts
JOIN article_archive a ON a.rowid = article_fts.rowid
JOIN feed_items fi ON fi.item_url = a.key
WHERE article_fts.doc MATCH ? AND fi.feed_url = ?
ORDER BY article_fts.rank
LIMIT ? OFFSET ?;

-- name: CountSearchFeedArchive :one
SELECT COUNT(*)
FROM article_fts
JOIN article_archive a ON a.rowid = article_fts.rowid
JOIN feed_items fi ON fi.item_url = a.key
WHERE article_fts.doc MATCH ? AND fi.feed_url = ?;
