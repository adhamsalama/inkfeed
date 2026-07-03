package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestQueries(t *testing.T) *Queries {
	t.Helper()
	dir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	return New(sqlDB)
}

func TestUserAndSessionQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()

	u, err := q.CreateUser(ctx, CreateUserParams{Email: "a@b.com", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 || u.Email != "a@b.com" {
		t.Fatalf("unexpected user %+v", u)
	}

	// Duplicate email should error (UNIQUE).
	if _, err := q.CreateUser(ctx, CreateUserParams{Email: "a@b.com", PasswordHash: "x"}); err == nil {
		t.Error("expected duplicate email error")
	}

	byEmail, err := q.GetUserByEmail(ctx, "a@b.com")
	if err != nil || byEmail.ID != u.ID {
		t.Fatalf("GetUserByEmail: %v %+v", err, byEmail)
	}
	if _, err := q.GetUserByEmail(ctx, "missing@b.com"); err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}

	byID, err := q.GetUserByID(ctx, u.ID)
	if err != nil || byID.Email != "a@b.com" {
		t.Fatalf("GetUserByID: %v", err)
	}

	if err := q.UpdateUserPassword(ctx, UpdateUserPasswordParams{PasswordHash: "newhash", ID: u.ID}); err != nil {
		t.Fatal(err)
	}
	byID, _ = q.GetUserByID(ctx, u.ID)
	if byID.PasswordHash != "newhash" {
		t.Errorf("password not updated")
	}

	// Sessions
	exp := time.Now().Add(time.Hour)
	if err := q.CreateSession(ctx, CreateSessionParams{Token: "tok", UserID: u.ID, ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	s, err := q.GetSession(ctx, "tok")
	if err != nil || s.UserID != u.ID {
		t.Fatalf("GetSession: %v", err)
	}
	if err := q.DeleteSession(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetSession(ctx, "tok"); err != sql.ErrNoRows {
		t.Errorf("session should be deleted, got %v", err)
	}
	// A future-dated session is retrievable.
	q.CreateSession(ctx, CreateSessionParams{Token: "future", UserID: u.ID, ExpiresAt: time.Now().Add(24 * time.Hour)})
	if _, err := q.GetSession(ctx, "future"); err != nil {
		t.Errorf("valid session should be returned, got %v", err)
	}
}

func TestPreferencesQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()
	u, _ := q.CreateUser(ctx, CreateUserParams{Email: "p@b.com", PasswordHash: "h"})

	if _, err := q.GetUserPreferences(ctx, u.ID); err != sql.ErrNoRows {
		t.Errorf("expected no prefs yet, got %v", err)
	}

	params := UpsertUserPreferencesParams{
		UserID:          u.ID,
		FontSize:        sql.NullFloat64{Float64: 1.2, Valid: true},
		LetterSpacing:   sql.NullFloat64{Float64: 0.1, Valid: true},
		LineHeight:      sql.NullFloat64{Float64: 1.5, Valid: true},
		CorsProxyUrl:    sql.NullString{String: "http://p", Valid: true},
		EpubEmbedImages: sql.NullInt64{Int64: 1, Valid: true},
		MobiEmbedImages: sql.NullInt64{Int64: 0, Valid: true},
		EmailTo:         sql.NullString{String: "k@x.com", Valid: true},
		FontFamily:      sql.NullString{String: "serif", Valid: true},
		BoldText:        sql.NullInt64{Int64: 1, Valid: true},
		DarkMode:        sql.NullInt64{Int64: 1, Valid: true},
	}
	if err := q.UpsertUserPreferences(ctx, params); err != nil {
		t.Fatal(err)
	}
	got, err := q.GetUserPreferences(ctx, u.ID)
	if err != nil || got.FontSize.Float64 != 1.2 || got.EmailTo.String != "k@x.com" {
		t.Fatalf("GetUserPreferences: %v %+v", err, got)
	}

	// Upsert again updates in place.
	params.FontSize = sql.NullFloat64{Float64: 2.0, Valid: true}
	q.UpsertUserPreferences(ctx, params)
	got, _ = q.GetUserPreferences(ctx, u.ID)
	if got.FontSize.Float64 != 2.0 {
		t.Errorf("upsert did not update font size")
	}
}

func TestSavedFeedsAndGroupsQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()
	u, _ := q.CreateUser(ctx, CreateUserParams{Email: "f@b.com", PasswordHash: "h"})

	q.InsertUserSavedFeed(ctx, InsertUserSavedFeedParams{UserID: u.ID, Url: "u1", Title: "T1", Position: 0, ArchiveEnabled: 1})
	q.InsertUserSavedFeed(ctx, InsertUserSavedFeedParams{UserID: u.ID, Url: "u2", Title: "T2", Position: 1, ArchiveEnabled: 0})
	feeds, err := q.GetUserSavedFeeds(ctx, u.ID)
	if err != nil || len(feeds) != 2 {
		t.Fatalf("GetUserSavedFeeds: %v %d", err, len(feeds))
	}
	if feeds[0].Url != "u1" || feeds[0].ArchiveEnabled != 1 {
		t.Errorf("unexpected feed %+v", feeds[0])
	}

	// GetDistinctSavedFeedURLs returns only archive-enabled feeds (u1).
	urls, err := q.GetDistinctSavedFeedURLs(ctx)
	if err != nil || len(urls) != 1 || urls[0] != "u1" {
		t.Fatalf("GetDistinctSavedFeedURLs: %v %v", err, urls)
	}

	if err := q.DeleteUserSavedFeeds(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	feeds, _ = q.GetUserSavedFeeds(ctx, u.ID)
	if len(feeds) != 0 {
		t.Errorf("feeds not deleted")
	}

	// Groups
	gid, err := q.InsertFeedGroup(ctx, InsertFeedGroupParams{UserID: u.ID, Name: "G", Position: 0})
	if err != nil {
		t.Fatal(err)
	}
	q.InsertFeedGroupItem(ctx, InsertFeedGroupItemParams{GroupID: gid, Url: "gu", Title: "gt", Position: 0})
	groups, err := q.GetUserFeedGroups(ctx, u.ID)
	if err != nil || len(groups) != 1 {
		t.Fatalf("GetUserFeedGroups: %v %d", err, len(groups))
	}
	items, err := q.GetFeedGroupItems(ctx, gid)
	if err != nil || len(items) != 1 || items[0].Url != "gu" {
		t.Fatalf("GetFeedGroupItems: %v %+v", err, items)
	}
	if err := q.DeleteUserFeedGroupItems(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteUserFeedGroups(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	groups, _ = q.GetUserFeedGroups(ctx, u.ID)
	if len(groups) != 0 {
		t.Errorf("groups not deleted")
	}
}

func TestFavoritesQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()
	u, _ := q.CreateUser(ctx, CreateUserParams{Email: "fav@b.com", PasswordHash: "h"})

	q.InsertUserFavorite(ctx, InsertUserFavoriteParams{UserID: u.ID, Url: "url", Title: "t", FeedTitle: "ft", PubDate: "pd", CommentsUrl: "cu"})
	favs, err := q.GetUserFavorites(ctx, u.ID)
	if err != nil || len(favs) != 1 || favs[0].CommentsUrl != "cu" {
		t.Fatalf("GetUserFavorites: %v %+v", err, favs)
	}
	if err := q.DeleteAllUserFavorites(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	favs, _ = q.GetUserFavorites(ctx, u.ID)
	if len(favs) != 0 {
		t.Errorf("favorites not deleted")
	}
}

func TestArticleArchiveQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()

	if _, err := q.GetArticleArchive(ctx, "k"); err != sql.ErrNoRows {
		t.Errorf("expected no rows, got %v", err)
	}

	q.UpsertArticleArchive(ctx, UpsertArticleArchiveParams{Key: "k1", Title: "T1", Author: "A", SiteName: "S", CreatedAt: "2024", HtmlContent: "<p>hi</p>", TextContent: "hi"})
	row, err := q.GetArticleArchive(ctx, "k1")
	if err != nil || row.Title != "T1" {
		t.Fatalf("GetArticleArchive: %v %+v", err, row)
	}

	// Upsert updates content.
	q.UpsertArticleArchive(ctx, UpsertArticleArchiveParams{Key: "k1", Title: "T1b", HtmlContent: "<p>bye</p>", TextContent: "bye"})
	row, _ = q.GetArticleArchive(ctx, "k1")
	if row.Title != "T1b" {
		t.Errorf("upsert did not update title: %q", row.Title)
	}

	q.UpsertArticleArchive(ctx, UpsertArticleArchiveParams{Key: "k2", HtmlContent: "x", TextContent: "y"})
	size, err := q.GetArticleArchiveTotalSize(ctx)
	if err != nil || size <= 0 {
		t.Fatalf("GetArticleArchiveTotalSize: %v %d", err, size)
	}

	oldest, err := q.GetOldestArticleArchiveKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if oldest.Key == "" {
		t.Errorf("empty oldest key")
	}
	if err := q.DeleteOldestArticleArchiveRow(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFeedItemsQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()

	if _, err := q.GetNextFeedItemWithoutArchive(ctx); err != sql.ErrNoRows {
		t.Errorf("expected no rows, got %v", err)
	}

	res, err := q.InsertFeedItem(ctx, InsertFeedItemParams{
		FeedUrl: "feed", ItemUrl: "item1", Title: "T", Description: "D", PubDate: "2024-01-01T00:00:00Z",
		CommentsUrl: sql.NullString{String: "c", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Errorf("expected 1 row inserted, got %d", n)
	}
	// Duplicate (feed_url,item_url) is ignored -> 0 rows.
	res, _ = q.InsertFeedItem(ctx, InsertFeedItemParams{FeedUrl: "feed", ItemUrl: "item1", Title: "T"})
	if n, _ := res.RowsAffected(); n != 0 {
		t.Errorf("expected duplicate ignored, got %d rows", n)
	}

	next, err := q.GetNextFeedItemWithoutArchive(ctx)
	if err != nil || next != "item1" {
		t.Fatalf("GetNextFeedItemWithoutArchive: %v %q", err, next)
	}

	items, err := q.GetFeedArchiveItems(ctx, GetFeedArchiveItemsParams{FeedUrl: "feed", Limit: 10, Offset: 0})
	if err != nil || len(items) != 1 {
		t.Fatalf("GetFeedArchiveItems: %v %d", err, len(items))
	}
	total, err := q.CountFeedArchiveItems(ctx, "feed")
	if err != nil || total != 1 {
		t.Fatalf("CountFeedArchiveItems: %v %d", err, total)
	}

	if err := q.MarkFeedItemArchiveFailed(ctx, "item1"); err != nil {
		t.Fatal(err)
	}
	// After marking failed, it's no longer the next-to-archive.
	if _, err := q.GetNextFeedItemWithoutArchive(ctx); err != sql.ErrNoRows {
		t.Errorf("expected no rows after mark failed, got %v", err)
	}

	// DeleteOldFeedItems with 0-hour threshold deletes nothing recent.
	if _, err := q.DeleteOldFeedItems(ctx, sql.NullString{String: "9999", Valid: true}); err != nil {
		t.Fatal(err)
	}
}

func TestWithTx(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()
	dir := t.TempDir()
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	schema, _ := os.ReadFile("schema.sql")
	sqlDB.Exec(string(schema))
	qt := New(sqlDB)

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	txq := qt.WithTx(tx)
	if _, err := txq.CreateUser(ctx, CreateUserParams{Email: "tx@b.com", PasswordHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := qt.GetUserByEmail(ctx, "tx@b.com"); err != nil {
		t.Errorf("committed tx user missing: %v", err)
	}
	_ = q
}

func TestRateLimitQueries(t *testing.T) {
	q := newTestQueries(t)
	ctx := context.Background()

	if _, err := q.GetIPRateLimit(ctx, GetIPRateLimitParams{Ip: "1.2.3.4", Endpoint: "signin"}); err != sql.ErrNoRows {
		t.Errorf("expected no rows, got %v", err)
	}

	now := time.Now()
	q.UpsertIPRateLimit(ctx, UpsertIPRateLimitParams{Ip: "1.2.3.4", Endpoint: "signin", Count: 1, WindowStart: now})
	row, err := q.GetIPRateLimit(ctx, GetIPRateLimitParams{Ip: "1.2.3.4", Endpoint: "signin"})
	if err != nil || row.Count != 1 {
		t.Fatalf("GetIPRateLimit: %v %+v", err, row)
	}
	// Upsert again with block.
	q.UpsertIPRateLimit(ctx, UpsertIPRateLimitParams{Ip: "1.2.3.4", Endpoint: "signin", Count: 5, WindowStart: now, BlockedUntil: sql.NullTime{Time: now.Add(time.Hour), Valid: true}})
	row, _ = q.GetIPRateLimit(ctx, GetIPRateLimitParams{Ip: "1.2.3.4", Endpoint: "signin"})
	if row.Count != 5 || !row.BlockedUntil.Valid {
		t.Errorf("upsert block failed: %+v", row)
	}
}
