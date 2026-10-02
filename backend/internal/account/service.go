package account

import (
	"context"
	"errors"
	"feedsystem_video_go/internal/auth"
	"log"
	"strconv"
	"strings"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountService struct {
	accountRepository *AccountRepository
	cache             *rediscache.Client
}

var (
	ErrUsernameTaken       = errors.New("username already exists")
	ErrNewUsernameRequired = errors.New("new_username is required")
)

func NewAccountService(accountRepository *AccountRepository, cache *rediscache.Client) *AccountService {
	return &AccountService{accountRepository: accountRepository, cache: cache}
}

// CreateAccount 注册账户：先将明文密码转为 bcrypt 哈希，再保存账户。
func (as *AccountService) CreateAccount(ctx context.Context, account *Account) error {
	// 数据库只保存不可逆的密码哈希；登录时校验哈希，不解密密码。
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(account.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	account.Password = string(passwordHash)
	if err := as.accountRepository.CreateAccount(ctx, account); err != nil {
		return err
	}
	return nil
}





// FindByID 按账户 ID 查询完整账户；密码和令牌依靠模型的 json:"-" 避免输出。
func (as *AccountService) FindByID(ctx context.Context, id uint) (*Account, error) {
	if account, err := as.accountRepository.FindByID(ctx, id); err != nil {
		return nil, err
	} else {
		return account, nil
	}
}

// FindByUsername 按用户名查询完整账户，也供登录和改密流程使用。
func (as *AccountService) FindByUsername(ctx context.Context, username string) (*Account, error) {
	if account, err := as.accountRepository.FindByUsername(ctx, username); err != nil {
		return nil, err
	} else {
		return account, nil
	}
}

// Login 校验密码并签发访问/刷新令牌；返回值依次为两种令牌和错误。
func (as *AccountService) Login(ctx context.Context, username, password string) (string, string, error) {
	account, err := as.FindByUsername(ctx, username)
	if err != nil {
		return "", "", err
	}
	// 使用 bcrypt 对比明文输入与已保存哈希，不直接比较字符串。
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(password)); err != nil {
		return "", "", err
	}
	accessToken, err := auth.GenerateToken(account.ID, account.Username)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := auth.GenerateRefreshToken(account.ID)
	if err != nil {
		return "", "", err
	}
	// 先持久化本次令牌，作为该账户当前会话的有效凭据。
	if err := as.accountRepository.Login(ctx, account.ID, accessToken, refreshToken); err != nil {
		return "", "", err
	}
	// 缓存账户到令牌的映射，以及刷新令牌到账户 ID 的反向映射；缓存写入是尽力而为。
	if as.cache != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		if err := as.cache.SetBytes(cacheCtx, as.cache.Key("account:%d", account.ID), []byte(accessToken), 24*time.Hour); err != nil {
			log.Printf("failed to set cache: %v", err)
		}
		if err := as.cache.SetBytes(cacheCtx, as.cache.Key("account:%d:refresh", account.ID), []byte(refreshToken), 7*24*time.Hour); err != nil {
			log.Printf("failed to set refresh cache: %v", err)
		}
		if err := as.cache.SetBytes(cacheCtx, as.cache.Key("refresh:%s", refreshToken), []byte(strconv.FormatUint(uint64(account.ID), 10)), 7*24*time.Hour); err != nil {
			log.Printf("failed to set refresh lookup: %v", err)
		}
	}
	return accessToken, refreshToken, nil
}

// Logout 清理账户当前访问令牌、刷新令牌及相应缓存。
func (as *AccountService) Logout(ctx context.Context, accountID uint) error {
	account, err := as.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	// 按当前实现，没有访问令牌时直接视为已退出。
	if account.Token == "" {
		return nil
	}
	// 删除两类会话缓存和刷新令牌反查项，最后由仓储清空数据库令牌。
	if as.cache != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		if err := as.cache.Del(cacheCtx, as.cache.Key("account:%d", account.ID)); err != nil {
			log.Printf("failed to del cache: %v", err)
		}
		if err := as.cache.Del(cacheCtx, as.cache.Key("account:%d:refresh", account.ID)); err != nil {
			log.Printf("failed to del refresh cache: %v", err)
		}
		if account.RefreshToken != "" {
			if err := as.cache.Del(cacheCtx, as.cache.Key("refresh:%s", account.RefreshToken)); err != nil {
				log.Printf("failed to del refresh lookup: %v", err)
			}
		}
	}
	return as.accountRepository.Logout(ctx, account.ID)
}



// FindAll 读取全部账户；当前用于刷新令牌缓存未命中时的数据库兜底。
func (as *AccountService) FindAll(ctx context.Context) ([]*Account, error) {
	return as.accountRepository.FindAll(ctx)
}



// RefreshAccessToken 根据当前保存的刷新令牌签发新访问令牌，不轮换刷新令牌。
func (as *AccountService) RefreshAccessToken(ctx context.Context, refreshToken string) (string, uint, string, error) {
	if refreshToken == "" {
		return "", 0, "", errors.New("refresh token is empty")
	}
	// 先用 Redis 反向映射快速定位账户；还要核对数据库中的当前刷新令牌，不能只信缓存。
	if as.cache != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		b, err := as.cache.GetBytes(cacheCtx, as.cache.Key("refresh:%s", refreshToken))
		if err == nil {
			idStr := string(b)
			id, parseErr := strconv.ParseUint(idStr, 10, 64)
			if parseErr == nil {
				account, err := as.FindByID(ctx, uint(id))
				if err == nil && account != nil && account.RefreshToken == refreshToken {
					newToken, err := auth.GenerateToken(account.ID, account.Username)
					if err != nil {
						return "", 0, "", err
					}
					if err := as.accountRepository.UpdateToken(ctx, account.ID, newToken); err != nil {
						return "", 0, "", err
					}
					if err := as.cache.SetBytes(cacheCtx, as.cache.Key("account:%d", account.ID), []byte(newToken), 24*time.Hour); err != nil {
						log.Printf("failed to set cache: %v", err)
					}
					return newToken, account.ID, account.Username, nil
				}
			}
		}
	}
	// 缓存未命中或核对失败时遍历数据库账户做精确匹配；当前路径不是按刷新令牌索引查询。
	accounts, err := as.FindAll(ctx)
	if err != nil {
		return "", 0, "", err
	}
	for _, acc := range accounts {
		if acc.RefreshToken == refreshToken {
			newToken, err := auth.GenerateToken(acc.ID, acc.Username)
			if err != nil {
				return "", 0, "", err
			}
			if err := as.accountRepository.UpdateToken(ctx, acc.ID, newToken); err != nil {
				return "", 0, "", err
			}
			return newToken, acc.ID, acc.Username, nil
		}
	}
	return "", 0, "", errors.New("invalid refresh token")
}
