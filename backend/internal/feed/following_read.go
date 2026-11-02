package feed

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/social"
	"feedsystem_video_go/internal/video"
)

// followingAuthors 以当前数据库关注关系划分来源；不维护第二份 following:bigv 缓存。
func (repo *FeedRepository) followingAuthors(ctx context.Context, viewer uint, threshold int64) (ordinary, big []uint, err error) {
	type authorCount struct {
		ID   uint
		Fans int64
	}
	var rows []authorCount
	counts := repo.db.WithContext(ctx).Model(&social.Social{}).Select("vlogger_id, COUNT(*) AS fans").Group("vlogger_id")
	err = repo.db.WithContext(ctx).Model(&social.Social{}).Select("socials.vlogger_id AS id, counts.fans").Joins("JOIN (?) AS counts ON counts.vlogger_id = socials.vlogger_id", counts).Where("socials.follower_id = ?", viewer).Order("socials.vlogger_id ASC").Scan(&rows).Error
	for _, r := range rows {
		if r.Fans >= threshold {
			big = append(big, r.ID)
		} else {
			ordinary = append(ordinary, r.ID)
		}
	}
	return
}

func (repo *FeedRepository) authorVideos(ctx context.Context, authors []uint, limit int, cursor *FollowingCursor) ([]*video.Video, error) {
	if len(authors) == 0 {
		return []*video.Video{}, nil
	}
	q := repo.db.WithContext(ctx).Where("author_id IN ?", authors).Order("create_time DESC, id DESC")
	if cursor != nil {
		q = q.Where("(create_time < ?) OR (create_time = ? AND id < ?)", cursor.Time, cursor.Time, cursor.ID)
	}
	var rows []*video.Video
	err := q.Limit(limit).Find(&rows).Error
	return rows, err
}

// cachedFollowingSource 只接受有完整性标记、且覆盖足够候选的窗口。
// 不足一页多一条时整路回源，不把缓存尾部当作真正的历史结束。
func (f *FeedService) cachedFollowingSource(ctx context.Context, key string, authors []uint, n int, cursor *FollowingCursor) ([]*video.Video, bool) {
	if f.rediscache == nil || len(authors) == 0 {
		return nil, false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	before, err := f.rediscache.GetBytes(cacheCtx, key+":ready")
	if err != nil || string(before) == "empty" {
		return nil, false
	}
	scope, err := f.rediscache.GetBytes(cacheCtx, key+":scope")
	if err != nil || string(scope) != fmt.Sprint(authors) {
		return nil, false
	}
	members, err := f.rediscache.ZRevRange(cacheCtx, key, 0, 999)
	if err != nil {
		return nil, false
	}
	ids := []uint{}
	entries := []FollowingCursor{}
	for _, member := range members {
		parts := strings.Split(member, ":")
		if len(parts) != 2 {
			return nil, false
		}
		at, err := time.Parse("20060102150405.000000000", parts[0])
		if err != nil {
			return nil, false
		}
		id, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || id == 0 || uint64(uint(id)) != id {
			return nil, false
		}
		entry := FollowingCursor{Time: at, ID: uint(id)}
		if cursor != nil && !followingNewer(*cursor, entry) {
			continue
		}
		ids = append(ids, entry.ID)
		entries = append(entries, entry)
		if len(ids) == n {
			break
		}
	}
	if len(ids) < n {
		return nil, false
	}
	rows, err := f.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, false
	}
	byID := map[uint]*video.Video{}
	allowed := map[uint]bool{}
	for _, id := range authors {
		allowed[id] = true
	}
	for _, v := range rows {
		byID[v.ID] = v
	}
	ordered := make([]*video.Video, 0, n)
	for _, entry := range entries {
		v := byID[entry.ID]
		if v == nil || !allowed[v.AuthorID] || !v.CreateTime.Equal(entry.Time) {
			return nil, false
		}
		ordered = append(ordered, v)
	}
	// 期间发生增量导致标记失效时，放弃本次缓存结果。
	after, err := f.rediscache.GetBytes(cacheCtx, key+":ready")
	if err != nil || string(after) != string(before) {
		return nil, false
	}
	return ordered, true
}

func (f *FeedService) followingCandidates(ctx context.Context, viewer uint, n int, cursor *FollowingCursor) ([]*video.Video, error) {
	if f.rediscache == nil {
		return f.repo.ListByFollowing(ctx, n, viewer, cursor)
	}
	ordinary, big, err := f.repo.followingAuthors(ctx, viewer, f.bigVThreshold)
	if err != nil {
		return nil, err
	}
	// 大量作者时保留单条 SQL 路径，避免按作者查询放大。
	if len(big) > 16 {
		return f.repo.ListByFollowing(ctx, n, viewer, cursor)
	}
	sources := make([][]*video.Video, 0, len(big)+1)
	if len(ordinary) > 0 {
		key := f.rediscache.Key("feed:inbox:%d", viewer)
		rows, ok := f.cachedFollowingSource(ctx, key, ordinary, n, cursor)
		if !ok {
			rows, err = f.repo.authorVideos(ctx, ordinary, n, cursor)
			if err != nil {
				return nil, err
			}
		}
		sources = append(sources, rows)
	}
	for _, author := range big {
		key := f.rediscache.Key("feed:user_videos:%d", author)
		rows, ok := f.cachedFollowingSource(ctx, key, []uint{author}, n, cursor)
		if !ok {
			// 登记后再查主库；发布事件同时进入临时区，完成时合并。
			cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			token, owned, beginErr := f.rediscache.BeginInboxRebuild(cacheCtx, key, 2*time.Second)
			cancel()
			rows, err = f.repo.authorVideos(ctx, []uint{author}, n, cursor)
			if err != nil {
				return nil, err
			}
			if owned && beginErr == nil {
				if cursor == nil {
					f.publishFollowingSource(ctx, key, token, []uint{author}, rows)
				}
				cleanup, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
				_ = f.rediscache.AbortInboxRebuild(cleanup, key, token)
				stop()
			}
		}
		sources = append(sources, rows)
	}
	streams := make([][]FollowingCursor, len(sources))
	byID := map[uint]*video.Video{}
	for i, rows := range sources {
		for _, v := range rows {
			streams[i] = append(streams[i], FollowingCursor{Time: v.CreateTime, ID: v.ID})
			byID[v.ID] = v
		}
	}
	merged := mergeFollowingStreams(streams, cursor, n)
	result := make([]*video.Video, 0, len(merged))
	for _, e := range merged {
		result = append(result, byID[e.ID])
	}
	return result, nil
}

func (f *FeedService) publishFollowingSource(ctx context.Context, key, token string, authors []uint, rows []*video.Video) {
	members := make([]string, len(rows))
	for i, v := range rows {
		members[i] = rediscache.FollowingMember(v.CreateTime, v.ID)
	}
	if ok, err := f.rediscache.FinishInboxRebuild(ctx, key, token, members, 1000, 7*24*time.Hour); err == nil && ok {
		_ = f.rediscache.SetBytes(ctx, key+":scope", []byte(fmt.Sprint(authors)), 7*24*time.Hour)
	}
}
