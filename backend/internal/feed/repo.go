package feed

import (
	"context"
	"feedsystem_video_go/internal/social"
	"feedsystem_video_go/internal/video"
	"time"

	"gorm.io/gorm"
)

type FeedRepository struct {
	db *gorm.DB
}

func NewFeedRepository(db *gorm.DB) *FeedRepository {
	return &FeedRepository{db: db}
}

func (repo *FeedRepository) ListLatest(ctx context.Context, limit int, latestBefore time.Time) ([]*video.Video, error) {
	var videos []*video.Video
	query := repo.db.WithContext(ctx).Model(&video.Video{}).
		Order("create_time DESC")
	if !latestBefore.IsZero() {
		query = query.Where("create_time < ?", latestBefore)
	}
	if err := query.Limit(limit).Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (repo *FeedRepository) ListLikesCountWithCursor(ctx context.Context, limit int, cursor *LikesCountCursor) ([]*video.Video, error) {
	var videos []*video.Video
	query := repo.db.WithContext(ctx).Model(&video.Video{}).
		Order("likes_count DESC, id DESC")

	if cursor != nil {
		query = query.Where(
			"(likes_count < ?) OR (likes_count = ? AND id < ?)",
			cursor.LikesCount,
			cursor.LikesCount, cursor.ID,
		)
	}

	if err := query.Limit(limit).Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (repo *FeedRepository) ListByFollowing(ctx context.Context, limit int, viewerAccountID uint, cursor *FollowingCursor) ([]*video.Video, error) {
	followingSubQuery := repo.db.WithContext(ctx).
		Model(&social.Social{}).
		Select("vlogger_id").
		Where("follower_id = ?", viewerAccountID)
	query := repo.db.WithContext(ctx).Model(&video.Video{}).
		Where("author_id IN (?)", followingSubQuery).
		Order("create_time DESC, id DESC")
	if cursor != nil {
		query = query.Where(
			"(create_time < ?) OR (create_time = ? AND id < ?)",
			cursor.Time, cursor.Time, cursor.ID,
		)
	}
	var videos []*video.Video
	if err := query.Limit(limit).Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (repo *FeedRepository) ListByPopularity(ctx context.Context, limit int, popularityBefore int64, timeBefore time.Time, idBefore uint) ([]*video.Video, error) {
	var videos []*video.Video
	query := repo.db.WithContext(ctx).Model(&video.Video{}).
		Order("popularity DESC, create_time DESC, id DESC")

	// 只有当游标完整提供时才加过滤（popularity 允许为 0）
	if !timeBefore.IsZero() && idBefore > 0 {
		query = query.Where(
			"(popularity < ?) OR (popularity = ? AND create_time < ?) OR (popularity = ? AND create_time = ? AND id < ?)",
			popularityBefore,
			popularityBefore, timeBefore,
			popularityBefore, timeBefore, idBefore,
		)
	}

	if err := query.Limit(limit).Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (repo *FeedRepository) GetByIDs(ctx context.Context, ids []uint) ([]*video.Video, error) {
	var videos []*video.Video
	if len(ids) == 0 {
		return videos, nil
	}
	if err := repo.db.WithContext(ctx).Model(&video.Video{}).
		Where("id IN ?", ids).Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (repo *FeedRepository) ListByTag(ctx context.Context, tagName string, limit int) ([]*video.Video, error) {
	var videos []*video.Video
	err := repo.db.WithContext(ctx).Model(&video.Video{}).Table("videos").
		Joins("JOIN video_tags ON video_tags.video_id = videos.id").
		Joins("JOIN tags ON tags.id = video_tags.tag_id").
		Where("tags.name = ?", tagName).
		Order("videos.create_time desc").
		Limit(limit).
		Find(&videos).Error
	return videos, err
}

// ListVideoIndexesByAuthors 为重建读取指定作者的近期索引；作者分类由 service 决定。
func (repo *FeedRepository) ListVideoIndexesByAuthors(ctx context.Context, authors []uint, limit int) ([]video.Video, error) {
	if len(authors) == 0 {
		return []video.Video{}, nil
	}
	var videos []video.Video
	err := repo.db.WithContext(ctx).Select("id", "create_time").Where("author_id IN ?", authors).Order("create_time DESC, id DESC").Limit(limit).Find(&videos).Error
	return videos, err
}

// ListFollowingAuthors 只查询关注作者及粉丝数，不在仓储层决定大 V 分类。
func (repo *FeedRepository) ListFollowingAuthors(ctx context.Context, viewer uint) ([]FollowingAuthor, error) {
	counts := repo.db.WithContext(ctx).Model(&social.Social{}).Select("vlogger_id, COUNT(*) AS follower_count").Group("vlogger_id")
	var authors []FollowingAuthor
	err := repo.db.WithContext(ctx).Model(&social.Social{}).
		Select("socials.vlogger_id AS id, counts.follower_count").
		Joins("JOIN (?) AS counts ON counts.vlogger_id = socials.vlogger_id", counts).
		Where("socials.follower_id = ?", viewer).Order("socials.vlogger_id ASC").Scan(&authors).Error
	return authors, err
}

// ListByAuthors 按作者集合与复合游标读取视频；调用方决定集合和候选数量。
func (repo *FeedRepository) ListByAuthors(ctx context.Context, authors []uint, limit int, cursor *FollowingCursor) ([]*video.Video, error) {
	if len(authors) == 0 {
		return []*video.Video{}, nil
	}
	query := repo.db.WithContext(ctx).Where("author_id IN ?", authors).Order("create_time DESC, id DESC")
	if cursor != nil {
		query = query.Where("(create_time < ?) OR (create_time = ? AND id < ?)", cursor.Time, cursor.Time, cursor.ID)
	}
	var videos []*video.Video
	err := query.Limit(limit).Find(&videos).Error
	return videos, err
}
