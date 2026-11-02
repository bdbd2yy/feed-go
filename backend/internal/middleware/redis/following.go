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
	return appendFollowingScript.Run(ctx, c.rdb, []string{key}, member, capacity, ttl.Milliseconds(), mode).Err()
}
