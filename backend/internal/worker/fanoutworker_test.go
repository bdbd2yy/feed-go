package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/video"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type fakeFanoutRepo struct {
	v         video.Video
	count     int64
	followers []uint
	err       error
	pages     int
	failAfter uint
}

func (r *fakeFanoutRepo) video(context.Context, uint) (*video.Video, error)  { return &r.v, r.err }
func (r *fakeFanoutRepo) followerCount(context.Context, uint) (int64, error) { return r.count, nil }
func (r *fakeFanoutRepo) followersAfter(_ context.Context, _ uint, after uint, limit int) ([]uint, error) {
	r.pages++
	if r.failAfter != 0 && after >= r.failAfter {
		return nil, errors.New("temporary DB failure")
	}
	ids := []uint{}
	for _, id := range r.followers {
		if id > after && len(ids) < limit {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func TestFanoutRoutingAndTTL(t *testing.T) {
	ctx := context.Background()
	for _, big := range []bool{false, true} {
		name := "ordinary"
		if big {
			name = "bigv_at_threshold"
		}
		t.Run(name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = raw.Close() })
			cache := rediscache.NewClient(raw, "test:")
			repo := &fakeFanoutRepo{v: video.Video{ID: 101, AuthorID: 1, CreateTime: time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)}, count: 2, followers: []uint{7, 8}}
			threshold := int64(3)
			if big {
				threshold = 2
			}
			w := NewFanoutWorker(nil, cache, threshold)
			w.repo = repo
			w.capacity = 2
			inbox := cache.Key("feed:inbox:%d", 7)
			if err := cache.AppendFollowingIndex(ctx, inbox, rediscache.FollowingMember(repo.v.CreateTime.Add(-time.Hour), 1), 2, time.Hour, false); err != nil {
				t.Fatal(err)
			}
			mr.FastForward(time.Minute)
			before := mr.TTL(inbox)
			for i := 0; i < 2; i++ {
				if err := w.ProcessVideo(ctx, 101); err != nil {
					t.Fatal(err)
				}
			}
			member := rediscache.FollowingMember(repo.v.CreateTime, 101)
			out, err := raw.ZRevRange(ctx, cache.Key("feed:user_videos:%d", 1), 0, -1).Result()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(out, []string{member}) {
				t.Fatalf("outbox=%v", out)
			}
			n, _ := raw.ZCard(ctx, inbox).Result()
			want := int64(2)
			if big {
				want = 1
			}
			if n != want {
				t.Fatalf("inbox count=%d want=%d", n, want)
			}
			if mr.TTL(inbox) != before {
				t.Fatal("publishing extended reader inactivity TTL")
			}
			if mr.Exists(cache.Key("feed:inbox:%d", 8)) {
				t.Fatal("publishing created an inactive reader inbox")
			}
			if big && repo.pages != 0 {
				t.Fatal("big V enumerated followers")
			}
			if ttl := mr.TTL(cache.Key("feed:user_videos:%d", 1)); ttl != w.ttl {
				t.Fatalf("outbox TTL=%s", ttl)
			}
		})
	}
}

func TestFanoutPartialFailureAndReplay(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = raw.Close() })
	cache := rediscache.NewClient(raw, "test:")
	repo := &fakeFanoutRepo{v: video.Video{ID: 9, AuthorID: 1, CreateTime: time.Now()}, count: 501, failAfter: 500}
	for i := uint(1); i <= 501; i++ {
		repo.followers = append(repo.followers, i)
	}
	// Only two active inboxes are needed to exercise both follower batches.
	for _, id := range []uint{1, 501} {
		if err := cache.AppendFollowingIndex(ctx, cache.Key("feed:inbox:%d", id), "old", 1000, time.Hour, false); err != nil {
			t.Fatal(err)
		}
	}
	w := NewFanoutWorker(nil, cache, 1000)
	w.repo = repo
	if err := w.ProcessVideo(ctx, 9); err == nil {
		t.Fatal("partial batch failure must propagate for Nack")
	}
	repo.failAfter = 0
	if err := w.ProcessVideo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{1, 501} {
		n, err := raw.ZCard(ctx, cache.Key("feed:inbox:%d", id)).Result()
		if err != nil || n != 2 {
			t.Fatalf("reader %d replay count=%d err=%v", id, n, err)
		}
	}
	repo.err = gorm.ErrRecordNotFound
	if err := w.ProcessVideo(ctx, 9); err != nil {
		t.Fatal("deleted video should not retry forever")
	}
	if err := w.ProcessVideo(ctx, 0); err == nil {
		t.Fatal("zero video ID accepted")
	}
}
