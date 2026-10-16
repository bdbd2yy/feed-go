package message

import "time"

// Message 私信持久化模型，也作为发送接口响应和会话列表元素。
type Message struct {
	ID        uint      `gorm:"primaryKey" json:"id"`                           // 私信唯一 ID
	FromID    uint      `gorm:"index:idx_message_from;not null" json:"from_id"` // 发送者账户 ID，从认证上下文获取
	ToID      uint      `gorm:"index:idx_message_to;not null" json:"to_id"`     // 接收者账户 ID
	Content   string    `gorm:"type:text;not null" json:"content"`              // 私信正文
	IsRead    bool      `gorm:"default:false" json:"is_read"`                   // 是否已读；当前发送和列表接口不会主动将其改为已读
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`               // 发送时间，接口序列化为时间字符串
}

// SendRequest 向指定用户发送私信；发送者由认证上下文确定。
type SendRequest struct {
	ToID    uint   `json:"to_id"`   // 私信接收者的账户 ID
	Content string `json:"content"` // 待发送的文本内容，去除两端空白后不能为空
}

// ListRequest 查询当前登录用户与另一用户的双向私信。
type ListRequest struct {
	PeerID uint `json:"peer_id"` // 会话另一方的账户 ID，不是消息 ID
}

// ListResponse 双向私信列表，当前按时间倒序返回最近最多 50 条。
type ListResponse struct {
	Messages []Message `json:"messages"` // 双方发送的私信；没有消息时返回空数组
}
