package feed

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"feedsystem_video_go/internal/social"
	"feedsystem_video_go/internal/video"
	"github.com/gin-gonic/gin"
	mysqldrv "github.com/go-sql-driver/mysql"
	gomysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func followingRequest(t *testing.T, h *FeedHandler, body string, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/feed/listByFollowing", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if authenticated {
		c.Set("accountID", uint(7))
	}
	h.ListByFollowing(c)
	return w
}

func TestFollowingRejectsInvalidRequests(t *testing.T) {
	// A nil service ensures invalid requests are rejected before any database access.
	h := NewFeedHandler(nil)
	for _, body := range []string{
		`{"cursor":{"time":"2026-01-01T00:00:00Z"}}`,
		`{"cursor":{"id":102}}`,
		`{"cursor":{}}`,
		`{"cursor":{"time":"invalid","id":102}}`,
		`{"cursor":{"time":"2026-01-01T00:00:00Z","id":-1}}`,
		`{"latest_time":123}`,
		`{"latest_time":0}`,
		`{`,
	} {
		if w := followingRequest(t, h, body, true); w.Code != 400 {
			t.Fatalf("%s: status = %d, body = %s", body, w.Code, w.Body.String())
		}
	}
	if w := followingRequest(t, h, `{"limit":2}`, false); w.Code != 401 {
		t.Fatalf("unauthenticated status = %d", w.Code)
	}
}

// Creates only an isolated, temporary database; never migrates the application's database.
func openFollowingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN not set; real MySQL integration test")
	}
	cfg, err := mysqldrv.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("following_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + name + "`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Errorf("cleanup test DB: %v", err)
		}
	})
	cfg.DBName = name
	db, err := gorm.Open(gomysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&video.Video{}, &video.Like{}, &social.Social{}); err != nil {
		t.Fatal(err)
	}
	// Preserve sub-millisecond precision to exercise JSON cursor round-trips.
	if err := db.Exec("ALTER TABLE videos MODIFY create_time DATETIME(6)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestFollowingPaginationMySQL(t *testing.T) {
	db := openFollowingTestDB(t)
	at := time.Date(2026, 1, 1, 10, 0, 0, 123456000, time.UTC)
	records := []video.Video{
		{ID: 102, AuthorID: 1, CreateTime: at},
		{ID: 101, AuthorID: 2, CreateTime: at},
		{ID: 200, AuthorID: 2, CreateTime: at.Add(-time.Minute)},
		{ID: 103, AuthorID: 1, CreateTime: at.Add(time.Microsecond)},
		{ID: 400, AuthorID: 3, CreateTime: at.Add(time.Hour)},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	relations := []social.Social{{FollowerID: 7, VloggerID: 1}, {FollowerID: 7, VloggerID: 2}}
	if err := db.Create(&relations).Error; err != nil {
		t.Fatal(err)
	}
	h := NewFeedHandler(NewFeedService(NewFeedRepository(db), video.NewLikeRepository(db), nil))
	for _, limit := range []int{1, 2, 4, 10} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			var cursor *FollowingCursor
			var ids []uint
			for page := 0; page < 10; page++ {
				body, err := json.Marshal(ListByFollowingRequest{Limit: limit, Cursor: cursor})
				if err != nil {
					t.Fatal(err)
				}
				w := followingRequest(t, h, string(body), true)
				if w.Code != 200 {
					t.Fatalf("status = %d: %s", w.Code, w.Body.String())
				}
				var resp ListByFollowingResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				for _, v := range resp.VideoList {
					ids = append(ids, v.ID)
				}
				if !resp.HasMore {
					if resp.NextCursor != nil {
						t.Fatal("terminal page has a cursor")
					}
					break
				}
				if len(resp.VideoList) != limit || resp.NextCursor == nil {
					t.Fatal("invalid continuation")
				}
				if resp.NextCursor.ID != resp.VideoList[len(resp.VideoList)-1].ID {
					t.Fatal("cursor must reference last returned item")
				}
				cursor = resp.NextCursor
			}
			if want := []uint{103, 102, 101, 200}; !reflect.DeepEqual(ids, want) {
				t.Fatalf("IDs = %v, want %v", ids, want)
			}
		})
	}
	// An unknown viewer must never fall back to an unfiltered public feed.
	rows, err := NewFeedRepository(db).ListByFollowing(context.Background(), 10, 8, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("no-follow viewer: rows=%v err=%v", rows, err)
	}
	svc := h.service
	if err := db.Where("follower_id = ? AND vlogger_id = ?", 7, 1).Delete(&social.Social{}).Error; err != nil {
		t.Fatal(err)
	}
	resp, err := svc.ListByFollowing(context.Background(), 10, nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range resp.VideoList {
		if v.Author.ID != 2 {
			t.Fatalf("unfollowed author's video returned: %d", v.ID)
		}
	}
	fresh := video.Video{ID: 500, AuthorID: 2, CreateTime: at.Add(time.Hour)}
	if err := db.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	resp, err = svc.ListByFollowing(context.Background(), 1, nil, 7)
	if err != nil || len(resp.VideoList) != 1 || resp.VideoList[0].ID != 500 {
		t.Fatalf("refresh must see newly published video: %+v, %v", resp, err)
	}
}
