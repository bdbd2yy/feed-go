package video

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type LikeRepository struct {
	db *gorm.DB
}

func NewLikeRepository(db *gorm.DB) *LikeRepository {
	return &LikeRepository{db: db}
}

func (r *LikeRepository) Like(ctx context.Context, like *Like) error {
	return r.db.WithContext(ctx).Create(like).Error
}

func (r *LikeRepository) Unlike(ctx context.Context, like *Like) error {
	return r.db.WithContext(ctx).
		Where("video_id = ? AND account_id = ?", like.VideoID, like.AccountID).
		Delete(&Like{}).Error
}

func (r *LikeRepository) LikeIgnoreDuplicate(ctx context.Context, like *Like) (created bool, err error) {
	if like == nil || like.VideoID == 0 || like.AccountID == 0 {
		return false, nil
	}
	err = r.db.WithContext(ctx).Create(like).Error
	if err == nil {
		return true, nil
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return false, nil
	}
	return false, err
}

func (r *LikeRepository) DeleteByVideoAndAccount(ctx context.Context, videoID, accountID uint) (deleted bool, err error) {
	if videoID == 0 || accountID == 0 {
		return false, nil
	}
	res := r.db.WithContext(ctx).
		Where("video_id = ? AND account_id = ?", videoID, accountID).
		Delete(&Like{})
	return res.RowsAffected > 0, res.Error
}

// ApplyLikeTx 在单个事务内插入点赞记录并累加点赞数与累计热度。
// 唯一键冲突表示该用户已点赞（消息重复消费）：不改变计数，返回 applied=false 且不报错。
func (r *LikeRepository) ApplyLikeTx(ctx context.Context, like *Like) (applied bool, err error) {
	if like == nil || like.VideoID == 0 || like.AccountID == 0 {
		return false, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(like).Error; err != nil {
			// MySQL 的唯一键冲突只终止当前语句、不终止事务；返回 nil 让事务
			// 提交为无变更，等价于幂等跳过，无需改用 ON DUPLICATE KEY UPDATE。
			if isDupKey(err) {
				return nil
			}
			return err
		}
		// 计数与点赞记录同事务提交，任一步失败都会回滚刚插入的记录。
		if err := tx.Model(&Video{}).Where("id = ?", like.VideoID).
			UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count + 1, 0)")).Error; err != nil {
			return err
		}
		if err := tx.Model(&Video{}).Where("id = ?", like.VideoID).
			UpdateColumn("popularity", gorm.Expr("GREATEST(popularity + 1, 0)")).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// ApplyUnlikeTx 在单个事务内删除点赞记录并扣减点赞数与累计热度。
// 没有可删除的点赞记录表示重复取消：不改变计数，返回 applied=false 且不报错。
func (r *LikeRepository) ApplyUnlikeTx(ctx context.Context, videoID, accountID uint) (applied bool, err error) {
	if videoID == 0 || accountID == 0 {
		return false, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("video_id = ? AND account_id = ?", videoID, accountID).Delete(&Like{})
		if res.Error != nil {
			return res.Error
		}
		// 删除结果才是最终状态依据：0 行说明已被其他请求取消，不能重复扣减计数。
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&Video{}).Where("id = ?", videoID).
			UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count - 1, 0)")).Error; err != nil {
			return err
		}
		if err := tx.Model(&Video{}).Where("id = ?", videoID).
			UpdateColumn("popularity", gorm.Expr("GREATEST(popularity - 1, 0)")).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *LikeRepository) IsLiked(ctx context.Context, videoID, accountID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Like{}).
		Where("video_id = ? AND account_id = ?", videoID, accountID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *LikeRepository) BatchGetLiked(ctx context.Context, videoIDs []uint, accountID uint) (map[uint]bool, error) {
	likeMap := make(map[uint]bool)
	if len(videoIDs) == 0 {
		return likeMap, nil
	}
	if accountID == 0 {
		return likeMap, nil
	}
	var likes []Like
	err := r.db.WithContext(ctx).Model(&Like{}).
		Where("video_id IN ? AND account_id = ?", videoIDs, accountID).
		Find(&likes).Error
	if err != nil {
		return nil, err
	}
	for _, like := range likes {
		likeMap[like.VideoID] = true
	}
	return likeMap, nil
}

func (r *LikeRepository) ListLikedVideos(ctx context.Context, accountID uint) ([]Video, error) {
	var videos []Video
	if accountID == 0 {
		return videos, nil
	}
	err := r.db.WithContext(ctx).
		Model(&Video{}).
		Joins("JOIN likes ON likes.video_id = videos.id").
		Where("likes.account_id = ?", accountID).
		Order("likes.created_at desc").
		Limit(200).
		Find(&videos).Error
	if err != nil {
		return nil, err
	}
	return videos, nil
}
