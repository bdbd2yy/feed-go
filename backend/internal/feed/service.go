package feed

import (
	"context"
	"encoding/json"
	rediscache "feedsystem_video_go/internal/middleware/redis"
	"feedsystem_video_go/internal/video"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	redis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type FeedService struct {
	repo         *FeedRepository
	likeRepo     *video.LikeRepository
	rediscache   *rediscache.Client
	localcache   *cache.Cache
	cacheTTL     time.Duration
	requestGroup singleflight.Group
}

// CachedFeedData 是公共视频数据的缓存载体，不包含查看者专属点赞状态；当前 service 未使用此结构。
type CachedFeedData struct {
	PublicVideos []video.Video `json:"public_videos"` // 可跨查看者复用的视频实体列表
}

func NewFeedService(repo *FeedRepository, likeRepo *video.LikeRepository, rediscache *rediscache.Client) *FeedService {
	return &FeedService{repo: repo, likeRepo: likeRepo, rediscache: rediscache, localcache: cache.New(3*time.Second, 5*time.Second), cacheTTL: 24 * time.Hour}
}

// GetVideoByIDs 按输入 ID 顺序取视频；采用本地缓存、Redis、MySQL 三级读取。
func (f *FeedService) GetVideoByIDs(ctx context.Context, videoIDs []uint) ([]*video.Video, error) {
	// GetVideoByIDs 批量获取视频信息
	// 采用 L1(本地缓存) -> L2(Redis) -> L3(MySQL) 三级架构
	if len(videoIDs) == 0 {
		return []*video.Video{}, nil
	}

	videoMap := make(map[uint]*video.Video)
	//L1:本地缓存
	var missedL1 []uint
	for _, id := range videoIDs {
		cacheKey := f.rediscache.Key("video:entity:%d", id)
		if f.localcache != nil {
			if v, found := f.localcache.Get(cacheKey); found {
				if data, ok := v.(video.Video); ok {
					videoMap[id] = &data
					continue
				}
			}
		}
		// 记录未命中的 ID，准备进入下一级缓存
		missedL1 = append(missedL1, id)
	}

	if len(missedL1) == 0 {
		return buildOrderedResult(videoIDs, videoMap), nil
	}

	//L2:redis
	var missedL2 []uint
	if len(missedL1) > 0 {
		cacheKeys := make([]string, len(missedL1))
		for i, id := range missedL1 {
			cacheKeys[i] = f.rediscache.Key("video:entity:%d", id)
		}

		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		results, err := f.rediscache.MGet(cacheCtx, cacheKeys...)
		cancel()

		if err == nil {
			for i, res := range results {
				id := missedL1[i]
				if res != nil {
					if str, ok := res.(string); ok {
						var v video.Video
						if err := json.Unmarshal([]byte(str), &v); err == nil {
							videoMap[id] = &v
							// 回写更新 L1 本地缓存
							if f.localcache != nil {
								f.localcache.Set(cacheKeys[i], v, 5*time.Second)
							}
							continue
						}
					}
				}
				missedL2 = append(missedL2, id)
			}
		} else {
			// 如果 Redis 挂了或者超时了，全部降级到 L3
			missedL2 = missedL1
			log.Printf("L2 Redis MGet 失败，全部降级到 MySQL: %v", err)
		}
	}

	if len(missedL2) == 0 {
		return buildOrderedResult(videoIDs, videoMap), nil
	}

	//L3:MySQL
	// 对剩余 ID 并发回源；singleflight 只合并同进程中同一视频的并发查询，不是跨实例锁。
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range missedL2 {
		wg.Add(1)
		go func(videoID uint) {
			defer wg.Done()
			sfKey := f.rediscache.Key("sf:entity:%d", videoID)

			v, err, _ := f.requestGroup.Do(sfKey, func() (interface{}, error) {
				videoList, err := f.repo.GetByIDs(ctx, []uint{videoID})

				if err != nil || len(videoList) == 0 {
					return nil, err
				}

				// 复制实体后回填公共缓存；此处不能缓存查看者相关的 is_liked。
				safeCopy := *videoList[0]
				cachekey := f.rediscache.Key("video:entity:%d", safeCopy.ID)
				if b, err := json.Marshal(safeCopy); err == nil {
					//异步回写redis
					go func(k string, b []byte) {
						setCtx, setCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
						defer setCancel()

						f.rediscache.SetBytes(setCtx, k, b, time.Hour)
					}(cachekey, b)
				}
				return videoList[0], err
			})

			if err == nil && v != nil {
				safeCopy := *(v.(*video.Video))
				mu.Lock()
				videoMap[id] = &safeCopy
				mu.Unlock()
				f.localcache.Set(f.rediscache.Key("video:entity:%d", safeCopy.ID), safeCopy, 5*time.Second)
			}
		}(id)
	}
	// 等待全部查询后恢复输入顺序；当前实现跳过不存在或读取失败的 ID，不把单条错误整体返回。
	wg.Wait()
	return buildOrderedResult(videoIDs, videoMap), nil
}

// 查询最新视频 (冷热分离 + 游标分页)
// ListLatest 查询最新流：热时间线走 Redis，超出其历史覆盖范围时回源 MySQL。
func (f *FeedService) ListLatest(ctx context.Context, limit int, latestBefore time.Time, viewerAccountID uint) (ListLatestResponse, error) {
	if f.rediscache == nil {
		return f.listLatestFromDB(ctx, limit, latestBefore, viewerAccountID)
	}

	// 获取 ZSET 中最老的一条数据
	zsetTail, err := f.rediscache.ZRangeWithScores(ctx, f.rediscache.Key("feed:global_timeline"), 0, 0)

	if err != nil {
		return f.listLatestFromDB(ctx, limit, latestBefore, viewerAccountID)
	}

	isZsetEmpty := len(zsetTail) == 0

	if isZsetEmpty {
		//全局静态锁：无视所有用户的不同时间戳游标
		sfKey := f.rediscache.Key("sf:fallback:global_timeline_rebuild")

		v, err, _ := f.requestGroup.Do(sfKey, func() (interface{}, error) {
			// 无视游标，直接去 MySQL 捞最新的 1000 条
			dbVideos, err := f.repo.ListLatest(ctx, 1000, time.Time{})
			if err != nil {
				return nil, err
			}
			if len(dbVideos) == 0 {
				return "EMPTY_DB", nil // 防无限递归
			}

			// 重建 ZSET
			bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var zElements []redis.Z
			for _, vid := range dbVideos {
				zElements = append(zElements, redis.Z{
					Score:  float64(vid.CreateTime.UnixMilli()),
					Member: fmt.Sprintf("%d", vid.ID),
				})
			}
			f.rediscache.ZAdd(bgCtx, f.rediscache.Key("feed:global_timeline"), zElements...)
			return "SUCCESS", nil
		})

		if err != nil {
			return ListLatestResponse{}, err
		}
		if v == "EMPTY_DB" {
			return ListLatestResponse{HasMore: false}, nil
		}

		// 让所有被阻塞的请求重新查一遍
		return f.ListLatest(ctx, limit, latestBefore, viewerAccountID)
	}

	// ZSET 尾部时间是热点覆盖下界，单位毫秒；更早的请求属于冷历史。
	watermark := int64(zsetTail[0].Score)
	reqTime := time.Now().UnixMilli()
	if !latestBefore.IsZero() {
		reqTime = latestBefore.UnixMilli()
	}

	var baseVideos []*video.Video

	if reqTime <= watermark {
		//冷数据降级查库

		// 针对个别用户的防并发（此时可以用时间戳做锁，因为冷尾流量极小）
		sfKey := f.rediscache.Key("sf:cold:listLatest:%d:%d", limit, reqTime)
		v, err, _ := f.requestGroup.Do(sfKey, func() (interface{}, error) {
			return f.repo.ListLatest(ctx, limit, latestBefore)
		})
		if err != nil {
			return ListLatestResponse{}, err
		}
		baseVideos = v.([]*video.Video)
		// 不回写 ZSET，防止冷数据污染热点时间线

	} else {
		// 热数据直接查redis
		maxScore := "+inf"
		if !latestBefore.IsZero() {
			maxScore = fmt.Sprintf("%d", reqTime-1) // 防重复
		}

		videoIDsStr, err := f.rediscache.ZRevRangeByScore(ctx, f.rediscache.Key("feed:global_timeline"), maxScore, "-inf", 0, int64(limit))
		if err != nil {
			return ListLatestResponse{}, err
		}

		var videoIDs []uint
		for _, idStr := range videoIDsStr {
			if id, err := strconv.ParseUint(idStr, 10, 64); err == nil {
				videoIDs = append(videoIDs, uint(id))
			}
		}

		if len(videoIDs) > 0 {
			baseVideos, err = f.GetVideoByIDs(ctx, videoIDs)
			if err != nil {
				return ListLatestResponse{}, err
			}
		}

		// 刚好击穿了冷热边界
		if len(baseVideos) < limit {
			remainLimit := limit - len(baseVideos) // 计算还差几个

			var coldCursor time.Time
			if len(baseVideos) > 0 {
				coldCursor = baseVideos[len(baseVideos)-1].CreateTime
			} else {
				coldCursor = latestBefore
			}

			sfKey := f.rediscache.Key("sf:stitch:listLatest:%d:%d", remainLimit, coldCursor.UnixMilli())
			v, err, _ := f.requestGroup.Do(sfKey, func() (interface{}, error) {
				return f.repo.ListLatest(ctx, remainLimit, coldCursor)
			})

			if err == nil {
				coldVideos := v.([]*video.Video)
				baseVideos = append(baseVideos, coldVideos...)
			}
		}
	}

	var nextTime int64
	if len(baseVideos) > 0 {
		// 将本页最后一条视频的时间作为下一次请求的游标
		nextTime = baseVideos[len(baseVideos)-1].CreateTime.UnixMilli()
	}
	// 这里未额外查一条：取满一页只是“可能还有”，不是精确的剩余数据判断。
	hasMore := len(baseVideos) == limit

	feedVideos, err := f.buildFeedVideos(ctx, baseVideos, viewerAccountID)
	if err != nil {
		return ListLatestResponse{}, err
	}

	return ListLatestResponse{
		VideoList: feedVideos,
		NextTime:  nextTime,
		HasMore:   hasMore,
	}, nil
}

// listLatestFromDB 最新流的纯数据库兜底，保持与缓存路径相同的毫秒分页协议。
func (f *FeedService) listLatestFromDB(ctx context.Context, limit int, latestBefore time.Time, viewerAccountID uint) (ListLatestResponse, error) {
	videos, err := f.repo.ListLatest(ctx, limit, latestBefore)
	if err != nil {
		return ListLatestResponse{}, err
	}
	feedVideos, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
	if err != nil {
		return ListLatestResponse{}, err
	}
	var nextTime int64
	if len(videos) > 0 {
		nextTime = videos[len(videos)-1].CreateTime.UnixMilli()
	}
	return ListLatestResponse{
		VideoList: feedVideos,
		NextTime:  nextTime,
		HasMore:   len(videos) == limit,
	}, nil
}

// 按照点赞数查询视频
// ListLikesCount 按点赞数降序、ID 降序读取；用末条视频的两项值定位下一页。
func (f *FeedService) ListLikesCount(ctx context.Context, limit int, cursor *LikesCountCursor, viewerAccountID uint) (ListLikesCountResponse, error) {
	// 复合游标避免相同点赞数的视频仅按点赞数分页时漏页；点赞变化仍可能改变榜单顺序。
	videos, err := f.repo.ListLikesCountWithCursor(ctx, limit, cursor)
	if err != nil {
		return ListLikesCountResponse{}, err
	}
	hasMore := len(videos) == limit
	feedVideos, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
	if err != nil {
		return ListLikesCountResponse{}, err
	}
	resp := ListLikesCountResponse{
		VideoList: feedVideos,
		HasMore:   hasMore,
	}
	if len(videos) > 0 {
		last := videos[len(videos)-1]
		nextLikesCountBefore := last.LikesCount
		nextIDBefore := last.ID
		resp.NextLikesCountBefore = &nextLikesCountBefore
		resp.NextIDBefore = &nextIDBefore
	}
	return resp, nil
}

// 按照关注列表查询视频
func (f *FeedService) ListByFollowing(ctx context.Context, limit int, latestBefore time.Time, viewerAccountID uint) (ListByFollowingResponse, error) {
	doListByFollowingFromDB := func() (ListByFollowingResponse, error) {
		videos, err := f.repo.ListByFollowing(ctx, limit, viewerAccountID, latestBefore)
		if err != nil {
			return ListByFollowingResponse{}, err
		}
		var nextTime int64
		if len(videos) > 0 {
			nextTime = videos[len(videos)-1].CreateTime.Unix()
		} else {
			nextTime = 0
		}
		hasMore := len(videos) == limit
		feedVideos, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
		if err != nil {
			return ListByFollowingResponse{}, err
		}
		resp := ListByFollowingResponse{
			VideoList: feedVideos,
			NextTime:  nextTime,
			HasMore:   hasMore,
		}
		return resp, nil
	}
	var cacheKey string
	if viewerAccountID != 0 && f.rediscache != nil {
		before := int64(0)
		if !latestBefore.IsZero() {
			before = latestBefore.Unix()
		}
		cacheKey = f.rediscache.Key("feed:listByFollowing:limit=%d:accountID=%d:before=%d", limit, viewerAccountID, before)
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		b, err := f.rediscache.GetBytes(cacheCtx, cacheKey)
		if err == nil {
			var cached ListByFollowingResponse
			if err := json.Unmarshal(b, &cached); err == nil {
				return cached, nil
			}
		} else if rediscache.IsMiss(err) { // 缓存未命中
			lockKey := "lock:" + cacheKey
			// 缓存未命中，尝试加锁
			token, locked, _ := f.rediscache.Lock(cacheCtx, lockKey, 500*time.Millisecond)
			if locked {
				defer func() { _ = f.rediscache.Unlock(context.Background(), lockKey, token) }()
				if b, err := f.rediscache.GetBytes(cacheCtx, cacheKey); err == nil {
					var cached ListByFollowingResponse
					if err := json.Unmarshal(b, &cached); err == nil {
						return cached, nil
					}
				} else { // 缓存未命中，从数据库中查询
					resp, err := doListByFollowingFromDB()
					if err != nil {
						return ListByFollowingResponse{}, err
					}
					if b, err := json.Marshal(resp); err == nil {
						_ = f.rediscache.SetBytes(cacheCtx, cacheKey, b, f.cacheTTL)
					}
					return resp, nil
				}
			} else {
				for i := 0; i < 5; i++ {
					time.Sleep(20 * time.Millisecond)
					if b, err := f.rediscache.GetBytes(cacheCtx, cacheKey); err == nil {
						var cached ListByFollowingResponse
						if err := json.Unmarshal(b, &cached); err == nil {
							return cached, nil
						}
					}
				}
			}
		}
	}

	resp, err := doListByFollowingFromDB()
	if err != nil {
		return ListByFollowingResponse{}, err
	}
	if cacheKey != "" {
		if b, err := json.Marshal(resp); err == nil {
			cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()
			_ = f.rediscache.SetBytes(cacheCtx, cacheKey, b, f.cacheTTL)
		}
	}
	return resp, nil
}

// ListByPopularity 优先读最近一小时热榜快照；失败时按数据库累计热度复合游标分页。
func (f *FeedService) ListByPopularity(ctx context.Context, limit int, reqAsOf int64, offset int, viewerAccountID uint, latestPopularity int64, latestBefore time.Time, latestIDBefore uint) (ListByPopularityResponse, error) {
	// Redis 热榜（稳定分页：as_of + offset）
	if f.rediscache != nil {
		asOf := time.Now().UTC().Truncate(time.Minute)
		// 如果请求带了上一页返回的 reqAsOf，就使用上一页的时间，而不是重新取当前时间
		// 你自己想想实际业务场景，按流行度排序的时候也是需要不断返回下一页的视频的啊
		if reqAsOf > 0 {
			asOf = time.Unix(reqAsOf, 0).UTC().Truncate(time.Minute)
		}

		// 累计截止 as_of 的 60 个分钟窗口；Redis 的近期热度不等于数据库的累计热度。
		const win = 60
		keys := make([]string, 0, win)
		for i := 0; i < win; i++ {
			keys = append(keys, f.rediscache.Key("hot:video:1m:%s", asOf.Add(-time.Duration(i)*time.Minute).Format("200601021504")))
		}

		dest := f.rediscache.Key("hot:video:merge:1m:%s", asOf.Format("200601021504")) // 快照key：同一个as_of页内复用
		opCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
		defer cancel()

		// 同一 as_of 在快照有效期内复用榜单，尽量避免翻页时新互动改变排名。
		exists, _ := f.rediscache.Exists(opCtx, dest)
		if !exists {
			_ = f.rediscache.ZUnionStore(opCtx, dest, keys, "SUM")
			_ = f.rediscache.Expire(opCtx, dest, 2*time.Minute) // 给翻页留时间
		}

		start := int64(offset)
		stop := start + int64(limit) - 1
		members, err := f.rediscache.ZRevRange(opCtx, dest, start, stop)
		// 已有快照的续页读到末尾就返回结束；第一页无榜单时允许回源数据库。
		if err == nil && len(members) == 0 {
			if offset > 0 {
				return ListByPopularityResponse{
					VideoList:  []FeedVideoItem{},
					AsOf:       asOf.Unix(),
					NextOffset: offset,
					HasMore:    false,
				}, nil
			}
		}
		if err == nil && len(members) > 0 {
			ids := make([]uint, 0, len(members))
			for _, m := range members {
				u, err := strconv.ParseUint(m, 10, 64)
				if err == nil && u > 0 {
					ids = append(ids, uint(u))
				}
			}

			videos, err := f.repo.GetByIDs(ctx, ids)
			if err == nil {
				byID := make(map[uint]*video.Video, len(videos))
				for _, v := range videos {
					byID[v.ID] = v
				}
				ordered := make([]*video.Video, 0, len(ids))
				for _, id := range ids {
					if v := byID[id]; v != nil {
						ordered = append(ordered, v)
					}
				}
				items, err := f.buildFeedVideos(ctx, ordered, viewerAccountID)
				if err != nil {
					return ListByPopularityResponse{}, err
				}
				resp := ListByPopularityResponse{
					VideoList:  items,
					AsOf:       asOf.Unix(),
					NextOffset: offset + len(items),
					HasMore:    len(items) == limit,
				}
				if len(ordered) > 0 {
					last := ordered[len(ordered)-1]
					nextPopularity := last.Popularity
					nextBefore := last.CreateTime
					nextID := last.ID
					// 即使本页来自 Redis，也返回末条视频的数据库游标供降级使用；两种排序不同，不承诺切换后绝不重复。
					resp.NextLatestPopularity = &nextPopularity
					resp.NextLatestBefore = &nextBefore
					resp.NextLatestIDBefore = &nextID
				}
				return resp, nil
			}
		}
	}

	// Redis 不可用或第一页榜单无数据时，按累计热度、创建时间、ID 三项降序查库。
	videos, err := f.repo.ListByPopularity(ctx, limit, latestPopularity, latestBefore, latestIDBefore)
	if err != nil {
		return ListByPopularityResponse{}, err
	}
	items, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
	if err != nil {
		return ListByPopularityResponse{}, err
	}
	resp := ListByPopularityResponse{
		VideoList:  items,
		AsOf:       0,
		NextOffset: 0,
		HasMore:    len(items) == limit,
	}
	if len(videos) > 0 {
		last := videos[len(videos)-1]
		nextPopularity := last.Popularity
		nextBefore := last.CreateTime
		nextID := last.ID
		resp.NextLatestPopularity = &nextPopularity
		resp.NextLatestBefore = &nextBefore
		resp.NextLatestIDBefore = &nextID
	}
	return resp, nil
}

// buildFeedVideos 将公共视频实体转为展示项，并批量补充当前查看者的点赞状态。
func (f *FeedService) buildFeedVideos(ctx context.Context, videos []*video.Video, viewerAccountID uint) ([]FeedVideoItem, error) {
	feedVideos := make([]FeedVideoItem, 0, len(videos))
	videoIDs := make([]uint, len(videos))
	for i, v := range videos {
		videoIDs[i] = v.ID
	}
	// 一次批量查询替代逐视频查询；不同查看者的 is_liked 不能混用公共实体缓存。
	likedMap, err := f.likeRepo.BatchGetLiked(ctx, videoIDs, viewerAccountID)
	if err != nil {
		return nil, err
	}
	for _, video := range videos {
		feedVideos = append(feedVideos, FeedVideoItem{
			ID:          video.ID,
			Author:      FeedAuthor{ID: video.AuthorID, Username: video.Username},
			Title:       video.Title,
			Description: video.Description,
			PlayURL:     video.PlayURL,
			CoverURL:    video.CoverURL,
			CreateTime:  video.CreateTime.Unix(),
			LikesCount:  video.LikesCount,
			IsLiked:     likedMap[video.ID],
		})
	}
	return feedVideos, nil
}

// buildOrderedResult 按榜单/时间线给出的 ID 顺序恢复实体顺序，跳过缺失条目。
func buildOrderedResult(orderedIDs []uint, dataMap map[uint]*video.Video) []*video.Video {
	res := make([]*video.Video, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if v, exits := dataMap[id]; exits && v != nil {
			res = append(res, v)
		}
	}
	return res
}

// ListByTag 查询指定话题的视频，并补充当前查看者的点赞状态。
func (f *FeedService) ListByTag(ctx context.Context, tagName string, limit int, viewerAccountID uint) ([]FeedVideoItem, error) {
	videos, err := f.repo.ListByTag(ctx, tagName, limit)
	if err != nil {
		return nil, err
	}
	return f.buildFeedVideos(ctx, videos, viewerAccountID)
}
