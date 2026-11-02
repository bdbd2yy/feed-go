package feed

import "container/heap"

// mergeFollowingStreams 归并已按 (Time DESC, ID DESC) 排序的候选流。
// 每路对应读者 inbox 或一个大 V 的作品列表；这里只处理索引，不读取视频详情。
// cursor 是全局严格边界，与候选来自哪一路无关。重复 ID 不占返回名额。
// 调用者可请求 limit+1 个唯一候选，用于判断 has_more。
// 本函数只能归并已提供的数据，不能证明上游缓存的完整性或还有没有更多批次。
func mergeFollowingStreams(streams [][]FollowingCursor, cursor *FollowingCursor, limit int) []FollowingCursor {
	result := make([]FollowingCursor, 0)
	if limit <= 0 {
		return result
	}

	// 堆内每路只放当前队头，不把所有候选都放进堆。
	heads := make(followingHeap, 0, len(streams))
	for source, stream := range streams {
		if len(stream) > 0 {
			heads = append(heads, followingHead{entry: stream[0], source: source})
		}
	}
	heap.Init(&heads)
	seen := make(map[uint]struct{})
	for heads.Len() > 0 && len(result) < limit {
		head := heap.Pop(&heads).(followingHead)
		// 即使该项重复或在游标之前，也要推进原来那一路，否则会漏掉其后续候选。
		next := head.position + 1
		if next < len(streams[head.source]) {
			heap.Push(&heads, followingHead{
				entry: streams[head.source][next], source: head.source, position: next,
			})
		}
		if cursor != nil && !followingNewer(*cursor, head.entry) {
			continue
		}
		if _, exists := seen[head.entry.ID]; exists {
			continue
		}
		seen[head.entry.ID] = struct{}{}
		result = append(result, head.entry)
	}
	return result
}

// followingNewer 是 MySQL ORDER BY create_time DESC, id DESC 的内存对应规则。
func followingNewer(a, b FollowingCursor) bool {
	if a.Time.Equal(b.Time) {
		return a.ID > b.ID
	}
	return a.Time.After(b.Time)
}

type followingHead struct {
	entry    FollowingCursor
	source   int
	position int
}

type followingHeap []followingHead

func (h followingHeap) Len() int           { return len(h) }
func (h followingHeap) Less(i, j int) bool { return followingNewer(h[i].entry, h[j].entry) }
func (h followingHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *followingHeap) Push(x any)        { *h = append(*h, x.(followingHead)) }
func (h *followingHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = followingHead{}
	*h = old[:len(old)-1]
	return last
}
