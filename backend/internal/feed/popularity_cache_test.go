package feed

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/video"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func popularityTestService(t *testing.T) (*FeedService, *goredis.Client, time.Time) {
	t.Helper()
	mr := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = raw.Close() })
	// No entity repository: a cache hit must not access MySQL. Guest likes need no DB.
	svc := NewFeedService(nil, video.NewLikeRepository(nil), rediscache.NewClient(raw, "test:"))
	return svc, raw, time.Now().UTC().Truncate(time.Minute)
}

func seedPopularitySnapshot(t *testing.T, svc *FeedService, raw *goredis.Client, at time.Time, ids ...string) {
	t.Helper()
	key := svc.rediscache.Key("hot:video:merge:1m:%s", at.Format("200601021504"))
	for i, id := range ids {
		if err := raw.ZAdd(context.Background(), key, goredis.Z{Score: float64(len(ids) - i), Member: id}).Err(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPopularityUsesEntityCaches(t *testing.T) {
	for _, layer := range []string{"L1", "L2"} {
		t.Run(layer, func(t *testing.T) {
			svc, raw, at := popularityTestService(t)
			seedPopularitySnapshot(t, svc, raw, at, "3", "1", "2")
			for _, id := range []uint{1, 2, 3} {
				v := video.Video{ID: id, Title: "cached", CreateTime: at, Popularity: int64(id)}
				key := svc.rediscache.Key("video:entity:%d", id)
				if layer == "L1" {
					svc.localcache.Set(key, v, time.Minute)
				} else {
					b, err := json.Marshal(v)
					if err != nil {
						t.Fatal(err)
					}
					if err := raw.Set(context.Background(), key, b, time.Hour).Err(); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Repeat with Redis entity keys removed to verify L2 warmed L1.
			for read := 0; read < 2; read++ {
				resp, err := svc.ListByPopularity(context.Background(), 3, at.Unix(), 0, 0, 0, time.Time{}, 0)
				if err != nil {
					t.Fatal(err)
				}
				var ids []uint
				for _, item := range resp.VideoList {
					ids = append(ids, item.ID)
					if item.IsLiked {
						t.Fatal("guest is liked")
					}
				}
				if !reflect.DeepEqual(ids, []uint{3, 1, 2}) {
					t.Fatalf("order = %v", ids)
				}
				if resp.AsOf != at.Unix() || resp.NextOffset != 3 || !resp.HasMore || resp.NextLatestIDBefore == nil || *resp.NextLatestIDBefore != 2 {
					t.Fatalf("invalid continuation: %+v", resp)
				}
				if err := raw.Del(context.Background(), svc.rediscache.Key("video:entity:%d", 1), svc.rediscache.Key("video:entity:%d", 2), svc.rediscache.Key("video:entity:%d", 3)).Err(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestPopularityOffsetConsumesInvalidMembers(t *testing.T) {
	svc, raw, at := popularityTestService(t)
	seedPopularitySnapshot(t, svc, raw, at, "invalid", "1", "2")
	for _, id := range []uint{1, 2} {
		svc.localcache.Set(svc.rediscache.Key("video:entity:%d", id), video.Video{ID: id, CreateTime: at}, time.Minute)
	}
	first, err := svc.ListByPopularity(context.Background(), 2, at.Unix(), 0, 0, 0, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.VideoList) != 1 || first.VideoList[0].ID != 1 || first.NextOffset != 2 || !first.HasMore {
		t.Fatalf("first page: %+v", first)
	}
	next, err := svc.ListByPopularity(context.Background(), 2, first.AsOf, first.NextOffset, 0, 0, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.VideoList) != 1 || next.VideoList[0].ID != 2 || next.NextOffset != 3 || next.HasMore {
		t.Fatalf("next page: %+v", next)
	}
}

func TestPopularityColdEntitiesMySQL(t *testing.T) {
	db := openFollowingTestDB(t)
	svc, raw, at := popularityTestService(t)
	svc.repo = NewFeedRepository(db)
	svc.likeRepo = video.NewLikeRepository(db)
	for _, id := range []uint{1, 2} {
		if err := db.Create(&video.Video{ID: id, CreateTime: at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&video.Like{VideoID: 1, AccountID: 7}).Error; err != nil {
		t.Fatal(err)
	}
	seedPopularitySnapshot(t, svc, raw, at, "2", "99", "1") // 99 is deleted/missing.
	resp, err := svc.ListByPopularity(context.Background(), 3, at.Unix(), 0, 7, 0, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.VideoList) != 2 || resp.VideoList[0].ID != 2 || resp.VideoList[1].ID != 1 || !resp.VideoList[1].IsLiked || resp.NextOffset != 3 || !resp.HasMore {
		t.Fatalf("cold page: %+v", resp)
	}
	// Public entity caches are reused across viewers, while likes remain viewer-specific.
	resp, err = svc.ListByPopularity(context.Background(), 3, at.Unix(), 0, 8, 0, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range resp.VideoList {
		if item.IsLiked {
			t.Fatal("viewer likes leaked")
		}
	}
}
