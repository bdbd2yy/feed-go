package video

import (
	"context"
	"strconv"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"
)

// UpdatePopularityCache 失效视频详情/实体缓存，并将热度增量写入当前 UTC 分钟窗口。
// 这里只维护 Redis 的近期热度，不修改数据库累计热度；缓存失败按尽力而为处理。
func UpdatePopularityCache(ctx context.Context, cache *rediscache.Client, id uint, change int64) {
	if cache == nil || id == 0 || change == 0 {
		return
	}

	// 删除过期的视频信息副本
	_ = cache.Del(context.Background(), cache.Key("video:detail:id=%d", id))
	_ = cache.Del(context.Background(), cache.Key("video:entity:%d", id))

	// 分钟窗口保留 2 小时，热榜读取时合并最近 60 个窗口形成约一小时的榜单。
	now := time.Now().UTC().Truncate(time.Minute)
	windowKey := cache.Key("hot:video:1m:%s", now.Format("200601021504"))
	member := strconv.FormatUint(uint64(id), 10)

	opCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_ = cache.ZincrBy(opCtx, windowKey, member, float64(change))
	_ = cache.Expire(opCtx, windowKey, 2*time.Hour)
}
