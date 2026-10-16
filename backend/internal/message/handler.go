package message

import (
	"context"
	"errors"
	"strings"
	"time"

	"feedsystem_video_go/internal/apierror"
	"feedsystem_video_go/internal/middleware/jwt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }
type Service struct{ repo *Repository }
type Handler struct{ service *Service }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func NewService(repo *Repository) *Service  { return &Service{repo: repo} }
func NewHandler(service *Service) *Handler  { return &Handler{service: service} }

func (r *Repository) AutoMigrate(ctx context.Context) error {
	return r.db.WithContext(ctx).AutoMigrate(&Message{})
}

// Send 去除正文两端空白，写入发送时间后保存私信；本模块的 Service 仅包装仓储。
func (r *Repository) Send(ctx context.Context, m *Message) error {
	m.Content = strings.TrimSpace(m.Content)
	if m.Content == "" {
		return errors.New("content is required")
	}
	m.CreatedAt = time.Now()
	return r.db.WithContext(ctx).Create(m).Error
}

// List 同时查询“我发给对方”和“对方发给我”，按时间倒序返回最近消息。
// 当前查询不会将消息标记为已读。
func (r *Repository) List(ctx context.Context, userID, peerID uint, limit int) ([]Message, error) {
	var msgs []Message
	err := r.db.WithContext(ctx).
		Where("(from_id = ? AND to_id = ?) OR (from_id = ? AND to_id = ?)", userID, peerID, peerID, userID).
		Order("created_at desc").
		Limit(limit).
		Find(&msgs).Error
	return msgs, err
}

func (h *Handler) Send(c *gin.Context) {
	fromID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ToID == 0 || strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to_id and content are required"})
		return
	}
	// 发送者只取认证身份，不能让客户端传入 FromID 冒充其他用户。
	m := &Message{FromID: fromID, ToID: req.ToID, Content: req.Content}
	if err := h.service.repo.Send(c.Request.Context(), m); err != nil {
		c.JSON(apierror.ClassifyHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *Handler) List(c *gin.Context) {
	userID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req ListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.PeerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_id is required"})
		return
	}
	msgs, err := h.service.repo.List(c.Request.Context(), userID, req.PeerID, 50)
	if err != nil {
		c.JSON(apierror.ClassifyHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []Message{}
	}
	c.JSON(http.StatusOK, ListResponse{Messages: msgs})
}
