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
			// source 是 streams 的下标；每路先放 stream[0]，所以 position 初始为 0。
			heads = append(heads, followingHead{entry: stream[0], source: source})
		}
	}
	// Init 根据 Less/Swap 把切片调整成堆；堆顶 heads[0] 是当前最新候选。
	// 堆只保证堆顶优先级最高，不代表整个 heads 切片已全局排好序。
	heap.Init(&heads)
	seen := make(map[uint]struct{})
	for heads.Len() > 0 && len(result) < limit {
		// 调用标准库 heap.Pop（不是直接调用 heads.Pop），它会维护堆结构。
		// 返回值类型为 any，用 .(followingHead) 类型断言取回实际候选。
		head := heap.Pop(&heads).(followingHead)
		// 即使该项重复或在游标之前，也要推进原来那一路，否则会漏掉其后续候选。
		// 例如 source=2、position=0，下一条就是 streams[2][1]。
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

// followingHead 是某一路当前参与比较的候选，并记录它在输入列表中的位置。
type followingHead struct {
	entry FollowingCursor // 当前候选，等于 streams[source][position]。
	// source 标识来自哪一路：streams[source]，不是作者 ID，也不是堆切片的下标。
	// 例如约定 streams[0] 为 inbox、streams[1] 为大 V 甲的发件箱，则 1 表示后者。
	source int
	// position 是候选在该路列表内的下标，从 0 开始；用于找到该路下一条。
	// 它不是客户端分页游标，也不会随堆内 Swap 改变。
	position int
}

// followingHeap 用切片保存堆。*followingHeap 实现 heap.Interface 的五个方法：
// Len、Less、Swap、Push、Pop；Go 通过方法集合隐式实现接口，无需 implements 声明。
// 标准库负责堆算法，我们提供长度、优先级、交换和切片增删操作。
type followingHeap []followingHead

func (h followingHeap) Len() int { return len(h) }

// 标准库优先弹出 Less 判断为“更小”的项；这里将“更新”定义为优先，形成最大优先堆。
func (h followingHeap) Less(i, j int) bool { return followingNewer(h[i].entry, h[j].entry) }

// 交换整个候选，source/position 随 entry 一起移动，始终指向原来的输入位置。
func (h followingHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push/Pop 使用指针接收者，因为增删会改变切片的长度，必须更新调用者持有的切片。
// 此 Push 只负责尾部追加；业务调用 heap.Push 后，标准库还会把新元素向上调整。
func (h *followingHeap) Push(x any) { *h = append(*h, x.(followingHead)) }

// 此 Pop 是标准库的回调，只负责删除并返回切片末项，不是直接取堆顶。
// 业务调用 heap.Pop 时，标准库先把堆顶移到末尾并调整剩余堆，再调用此方法。
func (h *followingHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = followingHead{} // 清空底层数组中不再使用的槽位。
	*h = old[:len(old)-1]
	return last
}
