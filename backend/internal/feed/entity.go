package feed

import "time"

// FeedAuthor 视频流中展示的作者摘要。
type FeedAuthor struct {
	ID       uint   `json:"id"`       // 视频作者的账户 ID
	Username string `json:"username"` // 视频中保存的作者用户名
}

// FeedVideoItem 视频流展示项；点赞状态与当前查看者有关，不应作为公共数据缓存。
type FeedVideoItem struct {
	ID          uint       `json:"id"`                    // 视频唯一 ID
	Author      FeedAuthor `json:"author"`                // 发布视频的作者信息
	Title       string     `json:"title"`                 // 视频标题
	Description string     `json:"description,omitempty"` // 视频描述；为空时可省略
	PlayURL     string     `json:"play_url"`              // 视频播放地址
	CoverURL    string     `json:"cover_url"`             // 封面图片地址
	CreateTime  int64      `json:"create_time"`           // 视频创建时间，Unix 秒；注意最新流的分页游标使用毫秒
	LikesCount  int64      `json:"likes_count"`           // 累计点赞数量
	IsLiked     bool       `json:"is_liked"`              // 当前查看者是否点赞；不是视频是否曾被任何人点赞
}

// ListLatestRequest 按创建时间倒序查询最新流。
type ListLatestRequest struct {
	Limit      int   `json:"limit"`       // 本页最多返回条数；接口允许 1～50，其他值按 10 处理
	LatestTime int64 `json:"latest_time"` // 时间上界，Unix 毫秒；第一页传 0，后续传上一页 next_time，严格取此前视频
}

// ListLatestResponse 最新流分页结果；仅用时间游标，同毫秒视频存在漏页风险。
type ListLatestResponse struct {
	VideoList []FeedVideoItem `json:"video_list"` // 本页视频，按创建时间倒序
	NextTime  int64           `json:"next_time"`  // 本页最后一条视频的创建时间，Unix 毫秒；下一页作为 latest_time 传回
	HasMore   bool            `json:"has_more"`   // 本页是否取满 limit；取满仅表示可能还有数据，不保证下一页非空
}

// ListLikesCountRequest 按点赞数降序、视频 ID 降序分页；两个游标字段需一起提供或一起省略。
type ListLikesCountRequest struct {
	Limit            int    `json:"limit"`                        // 本页最多返回条数；接口允许 1～50，其他值按 10 处理
	LikesCountBefore *int64 `json:"likes_count_before,omitempty"` // 上一页末条视频的点赞数；指针区分未提供与合法的 0 赞
	IDBefore         *uint  `json:"id_before,omitempty"`          // 上一页末条视频的 ID，点赞数相同时作为排序分界；双 0 表示第一页
}

// LikesCountCursor 点赞榜的复合游标，排序为点赞数降序、视频 ID 降序。
type LikesCountCursor struct {
	LikesCount int64 // 游标视频的点赞数，允许为 0
	ID         uint  // 游标视频 ID；下一页取相同点赞数下 ID 更小的视频
}

// ListLikesCountResponse 点赞榜分页结果。
type ListLikesCountResponse struct {
	VideoList            []FeedVideoItem `json:"video_list"`                        // 本页视频，按点赞数降序、ID 降序
	NextLikesCountBefore *int64          `json:"next_likes_count_before,omitempty"` // 下一页 likes_count_before；需与 next_id_before 配套使用
	NextIDBefore         *uint           `json:"next_id_before,omitempty"`          // 下一页 id_before；与点赞数共同定位本页末条视频
	HasMore              bool            `json:"has_more"`                          // 本页是否取满 limit，仅表示可能还有下一页
}

type ListByFollowingRequest struct {
	Limit      int   `json:"limit"`
	LatestTime int64 `json:"latest_time"`
}

type ListByFollowingResponse struct {
	VideoList []FeedVideoItem `json:"video_list"`
	NextTime  int64           `json:"next_time"`
	HasMore   bool            `json:"has_more"`
}

// ListByPopularityRequest 热榜请求：优先使用 as_of/offset 读取 Redis 快照，失败时使用数据库复合游标。
type ListByPopularityRequest struct {
	Limit          int   `json:"limit"`                      // 本页最多返回条数；接口允许 1～50，其他值按 10 处理
	AsOf           int64 `json:"as_of"`                      // 快照时间，Unix 秒且对齐到分钟；第一页传 0，续页使用服务端返回的 as_of
	Offset         int   `json:"offset"`                     // 快照内从 0 开始的排名偏移；第一页传 0，续页使用 next_offset
	LatestIDBefore *uint `json:"latest_id_before,omitempty"` // 数据库兜底游标的视频 ID；与 latest_before 同时提供，并配合 latest_popularity

	// DB fallback 用（可选）
	LatestPopularity int64     `json:"latest_popularity"` // 数据库兜底游标的累计热度，允许为 0；不是 Redis 时间窗分值
	LatestBefore     time.Time `json:"latest_before"`     // 数据库兜底游标的创建时间，JSON 时间字符串；与 latest_id_before 配套提供
}

// ListByPopularityResponse 热榜分页结果，同时提供快照定位信息和数据库兜底游标。
type ListByPopularityResponse struct {
	VideoList  []FeedVideoItem `json:"video_list"`  // 本页视频；Redis 按近 60 个分钟窗的热度和排序，数据库按累计热度排序
	AsOf       int64           `json:"as_of"`       // 当前 Redis 快照时间，Unix 秒；为 0 表示本页走了数据库兜底
	NextOffset int             `json:"next_offset"` // 下一页的 Redis 排名偏移；数据库兜底时为 0
	HasMore    bool            `json:"has_more"`    // 本页是否取满 limit，仅表示可能还有下一页

	NextLatestPopularity *int64     `json:"next_latest_popularity,omitempty"` // 下一页数据库游标的累计热度，传回 latest_popularity
	NextLatestBefore     *time.Time `json:"next_latest_before,omitempty"`     // 下一页数据库游标的创建时间，传回 latest_before
	NextLatestIDBefore   *uint      `json:"next_latest_id_before,omitempty"`  // 下一页数据库游标的视频 ID，传回 latest_id_before
}
