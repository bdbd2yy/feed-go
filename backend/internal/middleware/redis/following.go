package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// FollowingMember 使用固定宽度 UTC 时间和 ID，令字典序与复合游标顺序一致。
// 全部 score 为 0，避免毫秒 score 丢精度或相同时间下按非补零 ID 排错。
func FollowingMember(at time.Time, id uint) string {
	return fmt.Sprintf("%s:%020d", at.UTC().Format("20060102150405.000000000"), id)
}

var appendFollowingScript = goredis.NewScript(`
do
 if redis.call('EXISTS', KEYS[2]) == 1 then
  redis.call('ZADD', KEYS[3], 0, ARGV[1])
  redis.call('ZREMRANGEBYRANK', KEYS[3], 0, -tonumber(ARGV[2])-1)
  redis.call('PEXPIRE', KEYS[3], redis.call('PTTL', KEYS[2]))
 end
 redis.call('DEL', KEYS[4])
end
if ARGV[4] == '1' and redis.call('EXISTS', KEYS[1]) == 0 then
 return 0
end
redis.call('ZADD', KEYS[1], 0, ARGV[1])
redis.call('ZREMRANGEBYRANK', KEYS[1], 0, -tonumber(ARGV[2])-1)
if ARGV[4] == '0' then
 redis.call('PEXPIRE', KEYS[1], ARGV[3])
end
return 1
`)

// AppendFollowingIndex 原子地追加与裁剪索引。existingOnly 用于 inbox：
// 缺失时不创建，已有时保留原 TTL，避免发布事件持续续期闲置用户的缓存。
// 作者发件箱允许创建并续期。此方法不声明索引已完整或重建已完成。
func (c *Client) AppendFollowingIndex(ctx context.Context, key, member string, capacity int, ttl time.Duration, existingOnly bool) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis client not initialized")
	}
	if capacity <= 0 || ttl < time.Millisecond {
		return errors.New("invalid following index bounds")
	}
	mode := "0"
	if existingOnly {
		mode = "1"
	}
	return appendFollowingScript.Run(ctx, c.rdb, []string{key, key + ":building", key + ":buffer", key + ":ready"}, member, capacity, ttl.Milliseconds(), mode).Err()
}

// 重建租约与临时增量区共用固定后缀。所有切换与发布路由均由 Lua 原子协调。
var beginInboxRebuild = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('DEL', KEYS[2])
return 1
`)
var finishInboxRebuild = goredis.NewScript(`
if redis.call('GET', KEYS[2]) ~= ARGV[1] then return 0 end
for i=4,#ARGV do redis.call('ZADD', KEYS[3], 0, ARGV[i]) end
redis.call('ZREMRANGEBYRANK', KEYS[3], 0, -tonumber(ARGV[2])-1)
redis.call('DEL', KEYS[1])
local tail = redis.call('ZRANGE', KEYS[3], 0, 0)
local bound = 'empty'
if #tail > 0 then
 redis.call('RENAME', KEYS[3], KEYS[1])
 redis.call('PEXPIRE', KEYS[1], ARGV[3])
 bound = tail[1]
end
redis.call('SET', KEYS[4], bound, 'PX', ARGV[3])
redis.call('DEL', KEYS[2])
return 1
`)
var abortInboxRebuild = goredis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1], KEYS[2])
return 1
`)

func (c *Client) BeginInboxRebuild(ctx context.Context, key string, lease time.Duration) (string, bool, error) {
	if c == nil || c.rdb == nil {
		return "", false, errors.New("redis client not initialized")
	}
	token, err := randToken(16)
	if err != nil {
		return "", false, err
	}
	n, err := beginInboxRebuild.Run(ctx, c.rdb, []string{key + ":building", key + ":buffer"}, token, lease.Milliseconds()).Int()
	return token, n == 1, err
}

// FinishInboxRebuild 合并查询快照与期间分发的新索引。租约过期的旧任务不能发布结果。
// 元数据只记录已建立窗口的下界，不承诺关注变化/身份迁移后的完整性；读取接入另行验证。
func (c *Client) FinishInboxRebuild(ctx context.Context, key, token string, members []string, capacity int, ttl time.Duration) (bool, error) {
	args := []any{token, capacity, ttl.Milliseconds()}
	for _, m := range members {
		args = append(args, m)
	}
	n, err := finishInboxRebuild.Run(ctx, c.rdb, []string{key, key + ":building", key + ":buffer", key + ":ready"}, args...).Int()
	return n == 1, err
}
func (c *Client) AbortInboxRebuild(ctx context.Context, key, token string) error {
	return abortInboxRebuild.Run(ctx, c.rdb, []string{key + ":building", key + ":buffer"}, token).Err()
}

// TouchInbox 仅由访问请求续期，不由发布事件续期。ready 缺失则触发重建。
func (c *Client) TouchInbox(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	script := goredis.NewScript(`
local bound = redis.call('GET', KEYS[2])
if not bound then return 0 end
if bound ~= 'empty' and redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('PEXPIRE', KEYS[1], ARGV[1])
redis.call('PEXPIRE', KEYS[2], ARGV[1])
return 1
`)
	n, err := script.Run(ctx, c.rdb, []string{key, key + ":ready"}, ttl.Milliseconds()).Int()
	return n == 1, err
}
