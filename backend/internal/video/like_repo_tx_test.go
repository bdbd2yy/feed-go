package video

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	mysqldrv "github.com/go-sql-driver/mysql"
	gomysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// openTestLikeDB 为单个测试创建临时 MySQL database 并迁移 videos/likes 表，测试结束后删除。
// 需要设置 TEST_MYSQL_DSN（建议不带库名），并要求该账号具备 CREATE/DROP DATABASE 权限。
func openTestLikeDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}
	cfg, err := mysqldrv.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}

	adminCfg := cfg.Clone()
	adminCfg.DBName = ""
	admin, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		t.Fatalf("open mysql admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		t.Fatalf("ping mysql: %v", err)
	}

	dbName := fmt.Sprintf("like_tx_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4"); err != nil {
		_ = admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE `" + dbName + "`"); err != nil {
			t.Logf("drop test database %s: %v", dbName, err)
		}
		_ = admin.Close()
	})

	dbCfg := cfg.Clone()
	dbCfg.DBName = dbName
	db, err := gorm.Open(gomysql.Open(dbCfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&Video{}, &Like{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	return db
}

func seedTestVideo(t *testing.T, db *gorm.DB, likesCount, popularity int64) *Video {
	t.Helper()

	v := &Video{
		AuthorID:   1,
		Username:   "tester",
		Title:      "like tx test",
		PlayURL:    "/static/test.mp4",
		CoverURL:   "/static/test.jpg",
		LikesCount: likesCount,
		Popularity: popularity,
	}
	if err := db.Create(v).Error; err != nil {
		t.Fatalf("create video: %v", err)
	}
	return v
}

func countTestLikes(t *testing.T, db *gorm.DB, videoID, accountID uint) int64 {
	t.Helper()

	var count int64
	if err := db.Model(&Like{}).
		Where("video_id = ? AND account_id = ?", videoID, accountID).
		Count(&count).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	return count
}

func loadTestVideo(t *testing.T, db *gorm.DB, id uint) Video {
	t.Helper()

	var v Video
	if err := db.First(&v, id).Error; err != nil {
		t.Fatalf("load video %d: %v", id, err)
	}
	return v
}

func TestApplyLikeTx_CreatesLikeAndIncrementsCounters(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)

	applied, err := repo.ApplyLikeTx(context.Background(), &Like{
		VideoID:   v.ID,
		AccountID: 7,
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("ApplyLikeTx: %v", err)
	}
	if !applied {
		t.Fatal("expected applied=true for a new like")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 1 {
		t.Fatalf("like rows = %d, want 1", got)
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 1 || got.Popularity != 1 {
		t.Fatalf("likes_count/popularity = %d/%d, want 1/1", got.LikesCount, got.Popularity)
	}
}

func TestApplyLikeTx_DuplicateIsNoop(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)
	ctx := context.Background()

	if applied, err := repo.ApplyLikeTx(ctx, &Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()}); err != nil || !applied {
		t.Fatalf("first ApplyLikeTx = (%v, %v), want (true, nil)", applied, err)
	}
	applied, err := repo.ApplyLikeTx(ctx, &Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("duplicate ApplyLikeTx: %v", err)
	}
	if applied {
		t.Fatal("expected applied=false for a duplicate like")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 1 {
		t.Fatalf("like rows = %d, want 1", got)
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 1 || got.Popularity != 1 {
		t.Fatalf("likes_count/popularity = %d/%d, want 1/1", got.LikesCount, got.Popularity)
	}
}

func TestApplyLikeTx_InvalidInput(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)
	ctx := context.Background()

	cases := []struct {
		name string
		like *Like
	}{
		{name: "nil like", like: nil},
		{name: "zero video id", like: &Like{VideoID: 0, AccountID: 7}},
		{name: "zero account id", like: &Like{VideoID: v.ID, AccountID: 0}},
	}
	for _, tc := range cases {
		applied, err := repo.ApplyLikeTx(ctx, tc.like)
		if applied || err != nil {
			t.Fatalf("%s: ApplyLikeTx = (%v, %v), want (false, nil)", tc.name, applied, err)
		}
	}

	if applied, err := repo.ApplyUnlikeTx(ctx, 0, 7); applied || err != nil {
		t.Fatalf("ApplyUnlikeTx zero video = (%v, %v), want (false, nil)", applied, err)
	}
	if applied, err := repo.ApplyUnlikeTx(ctx, v.ID, 0); applied || err != nil {
		t.Fatalf("ApplyUnlikeTx zero account = (%v, %v), want (false, nil)", applied, err)
	}

	var total int64
	if err := db.Model(&Like{}).Count(&total).Error; err != nil {
		t.Fatalf("count all likes: %v", err)
	}
	if total != 0 {
		t.Fatalf("like rows = %d, want 0", total)
	}
}

func TestApplyLikeTx_RollsBackWhenCounterUpdateFails(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)

	// 删除 popularity 列，让事务的第三步失败，验证点赞记录与 likes_count 一起回滚。
	if err := db.Exec("ALTER TABLE videos DROP COLUMN popularity").Error; err != nil {
		t.Fatalf("drop popularity column: %v", err)
	}

	applied, err := repo.ApplyLikeTx(context.Background(), &Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error when popularity update fails")
	}
	if applied {
		t.Fatal("expected applied=false when the transaction rolls back")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 0 {
		t.Fatalf("like rows = %d, want 0 after rollback", got)
	}
	if got := loadTestVideo(t, db, v.ID); got.LikesCount != 0 {
		t.Fatalf("likes_count = %d, want 0 after rollback", got.LikesCount)
	}
}

func TestApplyUnlikeTx_DeletesAndDecrements(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)
	ctx := context.Background()

	if applied, err := repo.ApplyLikeTx(ctx, &Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()}); err != nil || !applied {
		t.Fatalf("ApplyLikeTx = (%v, %v), want (true, nil)", applied, err)
	}

	applied, err := repo.ApplyUnlikeTx(ctx, v.ID, 7)
	if err != nil {
		t.Fatalf("ApplyUnlikeTx: %v", err)
	}
	if !applied {
		t.Fatal("expected applied=true for an existing like")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 0 {
		t.Fatalf("like rows = %d, want 0", got)
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 0 || got.Popularity != 0 {
		t.Fatalf("likes_count/popularity = %d/%d, want 0/0", got.LikesCount, got.Popularity)
	}
}

func TestApplyUnlikeTx_MissingLikeIsNoop(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 5, 5)

	applied, err := repo.ApplyUnlikeTx(context.Background(), v.ID, 7)
	if err != nil {
		t.Fatalf("ApplyUnlikeTx: %v", err)
	}
	if applied {
		t.Fatal("expected applied=false when there is no like row")
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 5 || got.Popularity != 5 {
		t.Fatalf("likes_count/popularity = %d/%d, want 5/5", got.LikesCount, got.Popularity)
	}
}

func TestApplyUnlikeTx_ClampsAtZero(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)
	if err := db.Create(&Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("seed like: %v", err)
	}

	applied, err := repo.ApplyUnlikeTx(context.Background(), v.ID, 7)
	if err != nil {
		t.Fatalf("ApplyUnlikeTx: %v", err)
	}
	if !applied {
		t.Fatal("expected applied=true for an existing like")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 0 {
		t.Fatalf("like rows = %d, want 0", got)
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 0 || got.Popularity != 0 {
		t.Fatalf("likes_count/popularity = %d/%d, want 0/0", got.LikesCount, got.Popularity)
	}
}

func TestApplyUnlikeTx_RollsBackWhenCounterUpdateFails(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 1, 1)
	if err := db.Create(&Like{VideoID: v.ID, AccountID: 7, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("seed like: %v", err)
	}

	if err := db.Exec("ALTER TABLE videos DROP COLUMN popularity").Error; err != nil {
		t.Fatalf("drop popularity column: %v", err)
	}

	applied, err := repo.ApplyUnlikeTx(context.Background(), v.ID, 7)
	if err == nil {
		t.Fatal("expected error when popularity update fails")
	}
	if applied {
		t.Fatal("expected applied=false when the transaction rolls back")
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 1 {
		t.Fatalf("like rows = %d, want 1 after rollback", got)
	}
	if got := loadTestVideo(t, db, v.ID); got.LikesCount != 1 {
		t.Fatalf("likes_count = %d, want 1 after rollback", got.LikesCount)
	}
}

func TestApplyLikeTx_ConcurrentDuplicateOnlyIncrementsOnce(t *testing.T) {
	db := openTestLikeDB(t)
	repo := NewLikeRepository(db)
	v := seedTestVideo(t, db, 0, 0)

	const goroutines = 2
	applied := make([]bool, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			applied[i], errs[i] = repo.ApplyLikeTx(context.Background(), &Like{
				VideoID:   v.ID,
				AccountID: 7,
				CreatedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	appliedCount := 0
	for i := 0; i < goroutines; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d ApplyLikeTx: %v", i, errs[i])
		}
		if applied[i] {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("applied count = %d, want 1", appliedCount)
	}
	if got := countTestLikes(t, db, v.ID, 7); got != 1 {
		t.Fatalf("like rows = %d, want 1", got)
	}
	got := loadTestVideo(t, db, v.ID)
	if got.LikesCount != 1 || got.Popularity != 1 {
		t.Fatalf("likes_count/popularity = %d/%d, want 1/1", got.LikesCount, got.Popularity)
	}
}
