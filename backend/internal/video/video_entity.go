package video

import "time"

// Video 视频持久化模型；也用于发布、详情和作者作品列表的响应。
type Video struct {
	ID          uint      `gorm:"primaryKey" json:"id"`                                                                                                              // 视频唯一 ID
	AuthorID    uint      `gorm:"index;not null" json:"author_id"`                                                                                                   // 发布视频的账户 ID，从认证上下文获取
	Username    string    `gorm:"type:varchar(255);not null" json:"username"`                                                                                        // 发布时保存的作者用户名；不是实时关联查询出的用户名
	Title       string    `gorm:"type:varchar(255);not null" json:"title"`                                                                                           // 视频标题，可包含 #话题
	Description string    `gorm:"type:varchar(255);" json:"description,omitempty"`                                                                                   // 视频描述，可包含 #话题
	PlayURL     string    `gorm:"type:varchar(255);not null" json:"play_url"`                                                                                        // 已上传视频文件的播放地址
	CoverURL    string    `gorm:"type:varchar(255);not null" json:"cover_url"`                                                                                       // 已上传封面图片的访问地址
	CreateTime  time.Time `gorm:"autoCreateTime;index:idx_videos_create_time,sort:desc;index:idx_videos_popularity_time_id,priority:2,sort:desc" json:"create_time"` // 视频创建时间；直接返回该模型时序列化为时间字符串
	LikesCount  int64     `gorm:"column:likes_count;not null;default:0;index:idx_videos_likes_count_id,priority:1,sort:desc" json:"likes_count"`                     // 视频累计点赞数量
	Popularity  int64     `gorm:"column:popularity;not null;default:0;index:idx_videos_popularity_time_id,priority:1,sort:desc" json:"popularity"`                   // 数据库累计热度分值；点赞/评论增加分值，不等同于 Redis 最近一小时热榜分值
}

// PublishVideoRequest 发布视频元数据；应先上传视频与封面文件，作者身份由服务端补充。
type PublishVideoRequest struct {
	Title       string `json:"title"`       // 视频标题，不能为空白
	Description string `json:"description"` // 视频描述，可选；与标题一起用于提取 #话题
	PlayURL     string `json:"play_url"`    // 视频上传接口返回的播放地址
	CoverURL    string `json:"cover_url"`   // 封面上传接口返回的图片地址
}

// DeleteVideoRequest 删除当前登录用户自己的视频。
type DeleteVideoRequest struct {
	ID uint `json:"id"` // 要删除的视频 ID；service 会核对作者身份
}

// ListByAuthorIDRequest 查询指定作者的作品列表。
type ListByAuthorIDRequest struct {
	AuthorID uint `json:"author_id"` // 视频作者的账户 ID
}

// GetDetailRequest 查询单个视频详情。
type GetDetailRequest struct {
	ID uint `json:"id"` // 要查询的视频 ID
}

// UpdateLikesCountRequest 覆盖视频的点赞总数；不是点赞/取消点赞请求。
type UpdateLikesCountRequest struct {
	ID         uint  `json:"id"`          // 要更新的视频 ID
	LikesCount int64 `json:"likes_count"` // 新的点赞总数，按绝对值写入而非增量
}

type OutboxMsg struct {
	ID         uint      `gorm:"primaryKey"`
	VideoID    uint      `gorm:"index"`
	EventType  string    `gorm:"type:varchar(50)"`
	CreateTime time.Time `gorm:"autoCreateTime"`
	Status     string    `gorm:"type:varchar(50);index"`
}
