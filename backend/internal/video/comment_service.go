package video

import (
	"context"
	"errors"
	"feedsystem_video_go/internal/apierror"
	"feedsystem_video_go/internal/middleware/rabbitmq"
	rediscache "feedsystem_video_go/internal/middleware/redis"
	"log"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

type CommentService struct {
	repo            *CommentRepository
	VideoRepository *VideoRepository
	cache           *rediscache.Client
	commentMQ       *rabbitmq.CommentMQ
	popularityMQ    *rabbitmq.PopularityMQ
}

func NewCommentService(repo *CommentRepository, videoRepo *VideoRepository, cache *rediscache.Client, commentMQ *rabbitmq.CommentMQ, popularityMQ *rabbitmq.PopularityMQ) *CommentService {
	return &CommentService{repo: repo, VideoRepository: videoRepo, cache: cache, commentMQ: commentMQ, popularityMQ: popularityMQ}
}

// Publish 验证评论，优先异步保存评论并增加热度，消息失败时按路径兜底。
func (s *CommentService) Publish(ctx context.Context, comment *Comment) error {
	if comment == nil {
		return errors.New("comment is nil")
	}
	// 评论作者身份由 handler 补充；正文去掉两端空白后不能为空。
	comment.Username = strings.TrimSpace(comment.Username)
	comment.Content = strings.TrimSpace(comment.Content)
	if comment.VideoID == 0 || comment.AuthorID == 0 {
		return errors.New("video_id and author_id are required")
	}
	if comment.Content == "" {
		return errors.New("content is required")
	}

	exists, err := s.VideoRepository.IsExist(ctx, comment.VideoID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("video not found")
	}

	// 评论保存事件与 Redis 热度 +1 事件分别记录结果，不重复补偿已成功投递的消息。
	mysqlEnqueued := false
	redisEnqueued := false
	if s.commentMQ != nil {
		if err := s.commentMQ.Publish(ctx, comment.Username, comment.VideoID, comment.AuthorID, comment.Content); err == nil {
			mysqlEnqueued = true
		}
	}
	if s.popularityMQ != nil {
		if err := s.popularityMQ.Update(ctx, comment.VideoID, 1); err == nil {
			redisEnqueued = true
		}
	}
	// 两路投递成功即可返回并尝试通知；评论可能尚未落库，响应中的 ID/时间也可能尚未生成。
	if mysqlEnqueued && redisEnqueued {
		s.notifyMentions(ctx, comment)
		return nil
	}

	// 评论事件未成功投递时，直接事务保存评论并增加累计热度；成功入队的路径不重复写库。
	if !mysqlEnqueued {
		if err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Select("id").First(&Video{}, comment.VideoID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("video not found")
				}
				return err
			}
			// 直接写库时，把评论记录和累计热度 +1 放在同一事务，任一步失败则全部回滚。
			if err := tx.Create(comment).Error; err != nil {
				return err
			}
			return tx.Model(&Video{}).Where("id = ?", comment.VideoID).
				UpdateColumn("popularity", gorm.Expr("popularity + 1")).Error
		}); err != nil {
			return err
		}
	}

	// 热度事件未成功投递时，直接补 Redis 分钟热榜，不再次更新数据库累计热度。
	if !redisEnqueued {
		UpdatePopularityCache(ctx, s.cache, comment.VideoID, 1)
	}
	s.notifyMentions(ctx, comment)
	return nil
}

// Delete 只允许评论作者删除；优先交给 MQ，投递失败时直接删库。
func (s *CommentService) Delete(ctx context.Context, commentID uint, accountID uint) error {
	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return errors.New("comment not found")
	}
	// 核对的是评论作者，不是视频作者；当前流程删除评论时不扣减热度。
	if comment.AuthorID != accountID {
		return apierror.ErrUnauthorized
	}
	if s.commentMQ != nil {
		if err := s.commentMQ.Delete(ctx, commentID); err == nil {
			return nil
		}
	}
	return s.repo.DeleteComment(ctx, comment)
}

// GetAll 视频存在时返回它的评论；不存在的视频不能视为“评论为空”。
func (s *CommentService) GetAll(ctx context.Context, videoID uint) ([]Comment, error) {
	exists, err := s.VideoRepository.IsExist(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("video not found")
	}
	return s.repo.GetAllComments(ctx, videoID)
}

var mentionRegex = regexp.MustCompile(`@(\w+)`)

// notifyMentions 从评论中提取 @用户名 并写站内通知；失败不阻断评论发布。
func (s *CommentService) notifyMentions(ctx context.Context, comment *Comment) {
	// 当前正则的 \w 只匹配字母、数字、下划线等 ASCII 单词字符，不支持完整中文用户名。
	matches := mentionRegex.FindAllStringSubmatch(comment.Content, -1)
	if len(matches) == 0 {
		return
	}
	seen := make(map[string]bool)
	for _, m := range matches {
		username := m[1]
		// 同一评论对同名用户只通知一次，不通知评论作者本人，也跳过不存在的账户。
		if seen[username] || username == comment.Username {
			continue
		}
		seen[username] = true
		var accID uint
		if err := s.repo.db.WithContext(ctx).Table("accounts").Where("username = ?", username).Select("id").Scan(&accID).Error; err != nil || accID == 0 {
			continue
		}
		// 通知目标是所属视频，而不是评论 ID；异步评论此时可能还没有数据库 ID。
		notif := struct {
			RecipientID uint   // 被 @ 用户的账户 ID
			SenderID    uint   // 评论作者的账户 ID
			Type        string // 固定为 mention，表示评论提及
			TargetID    uint   // 评论所属视频 ID，通知点击后跳转到该视频
			Content     string // 通知列表展示文案，不是原始评论正文
		}{
			RecipientID: accID,
			SenderID:    comment.AuthorID,
			Type:        "mention",
			TargetID:    comment.VideoID,
			Content:     comment.Username + " 在评论中提到了你",
		}
		if err := s.repo.db.WithContext(ctx).Table("notifications").Create(&notif).Error; err != nil {
			log.Printf("create mention notification failed: %v", err)
		}
	}
}
