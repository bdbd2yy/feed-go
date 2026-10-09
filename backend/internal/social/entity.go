package social

import "feedsystem_video_go/internal/account"

// Social 有方向的关注关系：Follower 关注 Vlogger；两者都是账户。
type Social struct {
	ID         uint `gorm:"primaryKey"`                                                                 // 关注关系记录 ID
	FollowerID uint `gorm:"not null;index:idx_social_follower;uniqueIndex:idx_social_follower_vlogger"` // 发起关注的账户 ID，即粉丝/关注者
	VloggerID  uint `gorm:"not null;index:idx_social_vlogger;uniqueIndex:idx_social_follower_vlogger"`  // 被关注的账户 ID；不要求对方已发布视频
}

// FollowRequest 关注他人；关注者 ID 从当前登录用户获取。
type FollowRequest struct {
	VloggerID uint `json:"vlogger_id"` // 要关注的目标账户 ID
}

// UnfollowRequest 取消关注；关注者 ID 从当前登录用户获取。
type UnfollowRequest struct {
	VloggerID uint `json:"vlogger_id"` // 要取消关注的目标账户 ID
}

// GetAllFollowersRequest 查询某个账户的粉丝。
type GetAllFollowersRequest struct {
	VloggerID uint `json:"vlogger_id"` // 被查询账户 ID，返回关注这个账户的人
}

// GetAllFollowersResponse 粉丝列表及其数量。
type GetAllFollowersResponse struct {
	Followers     []*account.Account `json:"followers"`      // 关注目标账户的用户列表
	FollowerCount int64              `json:"follower_count"` // 目标账户的粉丝总数
}

// GetAllVloggersResponse 关注列表及其数量。
type GetAllVloggersResponse struct {
	Vloggers     []*account.Account `json:"vloggers"`      // 目标账户所关注的用户列表
	VloggerCount int64              `json:"vlogger_count"` // 目标账户关注的用户总数，不是视频数量
}

// SocialCounts 用户的粉丝数和关注数。
type SocialCounts struct {
	FollowerCount int64 `json:"follower_count"` // 关注该用户的人数
	VloggerCount  int64 `json:"vlogger_count"`  // 该用户关注的人数
}

// GetAllVloggersRequest 查询某个账户关注了谁。
type GetAllVloggersRequest struct {
	FollowerID uint `json:"follower_id"` // 被查询的关注者账户 ID，返回此人关注的用户
}
