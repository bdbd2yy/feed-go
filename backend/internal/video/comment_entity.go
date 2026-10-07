package video

import "time"

// Comment 视频评论模型，也直接作为评论列表的响应元素。
type Comment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`             // 评论唯一 ID
	Username  string    `gorm:"index" json:"username"`            // 发表评论时保存的用户名
	VideoID   uint      `gorm:"index" json:"video_id"`            // 评论所属的视频 ID
	AuthorID  uint      `gorm:"index" json:"author_id"`           // 评论作者的账户 ID，不是视频作者 ID
	Content   string    `gorm:"type:text" json:"content"`         // 评论正文；支持 @用户名 触发提及通知
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"` // 评论创建时间，接口序列化为时间字符串
}

// PublishCommentRequest 发表评论；评论作者 ID 和用户名由服务端补充。
type PublishCommentRequest struct {
	VideoID uint   `json:"video_id"` // 被评论的视频 ID
	Content string `json:"content"`  // 评论正文，不能全为空白；可包含 @用户名
}

// DeleteCommentRequest 删除当前登录用户自己发表的评论。
type DeleteCommentRequest struct {
	CommentID uint `json:"comment_id"` // 待删除的评论 ID，不是视频 ID
}

// GetAllCommentsRequest 查询某个视频的评论列表。
type GetAllCommentsRequest struct {
	VideoID uint `json:"video_id"` // 评论所属的视频 ID
}
