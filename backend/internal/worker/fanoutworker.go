package worker

import (
	"context"
	"errors"
	"feedsystem_video_go/internal/config"
	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/social"
	"feedsystem_video_go/internal/video"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// 小接口使分发规则能脱离 RabbitMQ、MySQL 连接进行验证。
type fanoutRepository interface {
	video(context.Context, uint) (*video.Video, error)
	followerCount(context.Context, uint) (int64, error)
	followersAfter(context.Context, uint, uint, int) ([]uint, error)
}

type fanoutDB struct{ db *gorm.DB }

func (r fanoutDB) video(ctx context.Context, id uint) (*video.Video, error) {
	var v video.Video
	err := r.db.WithContext(ctx).First(&v, id).Error
	return &v, err
}
func (r fanoutDB) followerCount(ctx context.Context, author uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&social.Social{}).Where("vlogger_id = ?", author).Count(&n).Error
	return n, err
}
func (r fanoutDB) followersAfter(ctx context.Context, author, after uint, limit int) ([]uint, error) {
	var ids []uint
	err := r.db.WithContext(ctx).Model(&social.Social{}).Where("vlogger_id = ? AND follower_id > ?", author, after).Order("follower_id ASC").Limit(limit).Pluck("follower_id", &ids).Error
	return ids, err
}

type FanoutWorker struct {
	repo          fanoutRepository
	cache         *rediscache.Client
	bigVThreshold int64
	capacity      int
	ttl           time.Duration
}

func NewFanoutWorker(db *gorm.DB, cache *rediscache.Client, threshold int64) *FanoutWorker {
	if threshold <= 0 {
		threshold = config.DefaultBigVThreshold
	}
	return &FanoutWorker{repo: fanoutDB{db}, cache: cache, bigVThreshold: threshold, capacity: 1000, ttl: 7 * 24 * time.Hour}
}

// ProcessVideo 使用数据库的作者和原始发布时间，不使用事件的毫秒时间重建精度。
// 部分粉丝写入后失败会整体重试；稳定 member 的 ZADD 使已成功部分保持状态幂等。
func (w *FanoutWorker) ProcessVideo(ctx context.Context, id uint) error {
	if id == 0 {
		return fmt.Errorf("video ID is required")
	}
	v, err := w.repo.video(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} // 已删除的视频不应无限重投。
	if err != nil {
		return err
	}
	member := rediscache.FollowingMember(v.CreateTime, v.ID)
	if err := w.cache.AppendFollowingIndex(ctx, w.cache.Key("feed:user_videos:%d", v.AuthorID), member, w.capacity, w.ttl, false); err != nil {
		return err
	}
	count, err := w.repo.followerCount(ctx, v.AuthorID)
	if err != nil {
		return err
	}
	if count >= w.bigVThreshold {
		return nil
	} // 大 V 仅维护发件箱，跳过逐粉丝写入。
	const batchSize = 500
	var after uint
	for {
		followers, err := w.repo.followersAfter(ctx, v.AuthorID, after, batchSize)
		if err != nil {
			return err
		}
		for _, follower := range followers {
			if err := w.cache.AppendFollowingIndex(ctx, w.cache.Key("feed:inbox:%d", follower), member, w.capacity, w.ttl, true); err != nil {
				return err
			}
		}
		if len(followers) < batchSize {
			return nil
		}
		after = followers[len(followers)-1]
	}
}
