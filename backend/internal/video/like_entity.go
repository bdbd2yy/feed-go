package video

import "time"

// Like 用户对视频的点赞记录；同一用户和视频组合唯一。
type Like struct {
	ID        uint      `gorm:"primaryKey" json:"id"`                                          // 点赞记录 ID
	VideoID   uint      `gorm:"uniqueIndex:idx_like_video_account;not null" json:"video_id"`   // 被点赞的视频 ID
	AccountID uint      `gorm:"uniqueIndex:idx_like_video_account;not null" json:"account_id"` // 点赞者的账户 ID，从认证上下文获取
	CreatedAt time.Time `json:"created_at"`                                                    // 该次点赞的时间
}

// LikeRequest 点赞、取消点赞或查询点赞状态的请求；用户身份由认证上下文确定。
type LikeRequest struct {
	VideoID uint `json:"video_id"` // 操作或查询的视频 ID
}
