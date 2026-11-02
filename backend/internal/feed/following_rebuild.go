package feed

import (
	"context"
	"errors"
	"log"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"
)

var ErrFollowingBusy = errors.New("following feed database concurrency limit reached")

// 不让冷缓存为每个请求创建一个排队 goroutine；后台重建也有独立并发上限。
// 当前缓存仍不用于接口读取，ready 只表示本次重建已发布，不能替代关系版本校验。
func (f *FeedService) maybeRebuildInbox(viewer uint) {
	if f.rediscache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	key := f.rediscache.Key("feed:inbox:%d", viewer)
	ready, err := f.rediscache.TouchInbox(ctx, key, 7*24*time.Hour)
	if err != nil || ready {
		return
	}
	select {
	case f.followingBuilds <- struct{}{}:
	default:
		return
	}
	token, ok, err := f.rediscache.BeginInboxRebuild(ctx, key, 10*time.Second)
	if err != nil || !ok {
		<-f.followingBuilds
		return
	}
	go func() {
		defer func() { <-f.followingBuilds }()
		buildCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := f.rebuildInbox(buildCtx, viewer, key, token); err != nil {
			log.Printf("following inbox rebuild failed: viewer=%d err=%v", viewer, err)
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = f.rediscache.AbortInboxRebuild(cleanup, key, token)
	}()
}

func (f *FeedService) rebuildInbox(ctx context.Context, viewer uint, key, token string) error {
	// 从主库在登记重建后读取普通作者作品；每次只取最近 1000 条。
	videos, err := f.repo.ListOrdinaryFollowing(ctx, viewer, f.bigVThreshold, 1000)
	if err != nil {
		return err
	}
	members := make([]string, len(videos))
	for i, v := range videos {
		members[i] = rediscache.FollowingMember(v.CreateTime, v.ID)
	}
	ok, err := f.rediscache.FinishInboxRebuild(ctx, key, token, members, 1000, 7*24*time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("inbox rebuild lease expired or superseded")
	}
	return nil
}
