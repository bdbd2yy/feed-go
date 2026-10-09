package social

import (
	"context"
	"errors"
	"feedsystem_video_go/internal/account"
	"feedsystem_video_go/internal/middleware/rabbitmq"
	rediscache "feedsystem_video_go/internal/middleware/redis"
	"log"
)

type SocialService struct {
	repo        *SocialRepository
	accountrepo *account.AccountRepository
	socialMQ    *rabbitmq.SocialMQ
	cache       *rediscache.Client
}

func NewSocialService(repo *SocialRepository, accountrepo *account.AccountRepository, socialMQ *rabbitmq.SocialMQ, cache *rediscache.Client) *SocialService {
	return &SocialService{repo: repo, accountrepo: accountrepo, socialMQ: socialMQ, cache: cache}
}

func (s *SocialService) Follow(ctx context.Context, social *Social) error {
	_, err := s.accountrepo.FindByID(ctx, social.FollowerID)
	if err != nil {
		return err
	}
	_, err = s.accountrepo.FindByID(ctx, social.VloggerID)
	if err != nil {
		return err
	}
	if social.FollowerID == social.VloggerID {
		return errors.New("can not follow self")
	}
	// 提前检查重复关注；并发请求的最终约束仍由数据库唯一键保证。
	isFollowed, err := s.repo.IsFollowed(ctx, social)
	if err != nil {
		return err
	}
	if isFollowed {
		return errors.New("already followed")
	}

	// 先写 DB，确保数据持久化
	if err := s.repo.Follow(ctx, social); err != nil {
		return err
	}

	// DB 成功后，失效该用户的关注列表缓存
	s.invalidateFollowingFeedCache(context.Background(), social.FollowerID)

	// 最后发 MQ（用于通知），失败只记日志不影响业务
	if s.socialMQ != nil {
		if err := s.socialMQ.Follow(ctx, social.FollowerID, social.VloggerID); err != nil {
			log.Printf("social MQ Follow 发布失败: %v", err)
		}
	}
	return nil
}

func (s *SocialService) Unfollow(ctx context.Context, social *Social) error {
	_, err := s.accountrepo.FindByID(ctx, social.FollowerID)
	if err != nil {
		return err
	}
	_, err = s.accountrepo.FindByID(ctx, social.VloggerID)
	if err != nil {
		return err
	}
	// 只有已有关系才能取关；取关者和目标 ID 的方向与关注一致。
	isFollowed, err := s.repo.IsFollowed(ctx, social)
	if err != nil {
		return err
	}
	if !isFollowed {
		return errors.New("not followed")
	}

	// 先写 DB
	if err := s.repo.Unfollow(ctx, social); err != nil {
		return err
	}

	// 失效缓存
	s.invalidateFollowingFeedCache(context.Background(), social.FollowerID)

	// 最后发 MQ
	if s.socialMQ != nil {
		if err := s.socialMQ.UnFollow(ctx, social.FollowerID, social.VloggerID); err != nil {
			log.Printf("social MQ UnFollow 发布失败: %v", err)
		}
	}
	return nil
}

func (s *SocialService) invalidateFollowingFeedCache(ctx context.Context, accountID uint) {
	if s.cache == nil {
		return
	}
	pattern := s.cache.Key("feed:listByFollowing:*:accountID=%d:*", accountID)
	if err := s.cache.DelByPattern(ctx, pattern); err != nil {
		log.Printf("失效 Following 缓存失败: accountID=%d, err=%v", accountID, err)
	}
}

// GetAllFollowers 查询目标账户的粉丝列表；先确认目标账户存在。
func (s *SocialService) GetAllFollowers(ctx context.Context, VloggerID uint) ([]*account.Account, error) {
	_, err := s.accountrepo.FindByID(ctx, VloggerID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetAllFollowers(ctx, VloggerID)
}

// GetAllVloggers 查询目标账户关注的用户列表；这里不是查询视频作者全集。
func (s *SocialService) GetAllVloggers(ctx context.Context, FollowerID uint) ([]*account.Account, error) {
	_, err := s.accountrepo.FindByID(ctx, FollowerID)
	if err != nil {
		return nil, err
	}
	return s.repo.GetAllVloggers(ctx, FollowerID)
}

func (s *SocialService) CountFollowers(ctx context.Context, vloggerID uint) (int64, error) {
	return s.repo.CountFollowers(ctx, vloggerID)
}

// CountVloggers 查询指定账户关注了多少人。
func (s *SocialService) CountVloggers(ctx context.Context, followerID uint) (int64, error) {
	return s.repo.CountVloggers(ctx, followerID)
}

// IsFollowed 确认双方账户存在后，检查 Follower 是否关注 Vlogger。
func (s *SocialService) IsFollowed(ctx context.Context, social *Social) (bool, error) {
	_, err := s.accountrepo.FindByID(ctx, social.FollowerID)
	if err != nil {
		return false, err
	}
	_, err = s.accountrepo.FindByID(ctx, social.VloggerID)
	if err != nil {
		return false, err
	}
	return s.repo.IsFollowed(ctx, social)
}
