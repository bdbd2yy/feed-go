package feed

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestMergeFollowingStreams(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 123456000, time.UTC)
	a := FollowingCursor{Time: at, ID: 102}
	b := FollowingCursor{Time: at, ID: 101}
	c := FollowingCursor{Time: at.Add(-time.Minute), ID: 200}
	d := FollowingCursor{Time: at.Add(time.Microsecond), ID: 103}
	e := FollowingCursor{Time: at.Add(-2 * time.Minute), ID: 99}
	cases := []struct {
		name    string
		streams [][]FollowingCursor
		cursor  *FollowingCursor
		limit   int
		want    []FollowingCursor
	}{
		{name: "no sources", limit: 2, want: []FollowingCursor{}},
		{name: "empty sources", streams: [][]FollowingCursor{nil, {}}, limit: 2, want: []FollowingCursor{}},
		{name: "inbox and two outboxes", streams: [][]FollowingCursor{{a, c}, {d, e}, {b}}, limit: 10, want: []FollowingCursor{d, a, b, c, e}},
		{name: "same time uses ID", streams: [][]FollowingCursor{{b}, {a}}, limit: 2, want: []FollowingCursor{a, b}},
		{name: "submillisecond order", streams: [][]FollowingCursor{{a}, {d}}, limit: 2, want: []FollowingCursor{d, a}},
		{name: "duplicate heads still advance", streams: [][]FollowingCursor{{a, b}, {a, c}, {a, e}}, limit: 3, want: []FollowingCursor{a, b, c}},
		{name: "duplicate within source", streams: [][]FollowingCursor{{a, a, b}}, limit: 2, want: []FollowingCursor{a, b}},
		{name: "cursor excludes itself and newer", streams: [][]FollowingCursor{{d, a, c}, {a, b, e}}, cursor: &a, limit: 3, want: []FollowingCursor{b, c, e}},
		{name: "cursor past all sources", streams: [][]FollowingCursor{{a}, {b}}, cursor: &e, limit: 3, want: []FollowingCursor{}},
		{name: "stop at requested unique count", streams: [][]FollowingCursor{{a, c}, {d, b}}, limit: 2, want: []FollowingCursor{d, a}},
		{name: "zero limit", streams: [][]FollowingCursor{{a}}, limit: 0, want: []FollowingCursor{}},
		{name: "negative limit", streams: [][]FollowingCursor{{a}}, limit: -1, want: []FollowingCursor{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := make([][]FollowingCursor, len(tc.streams))
			for i, stream := range tc.streams {
				if stream != nil {
					before[i] = append([]FollowingCursor{}, stream...)
				}
			}
			got := mergeFollowingStreams(tc.streams, tc.cursor, tc.limit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.streams {
				if !reflect.DeepEqual(tc.streams[i], before[i]) {
					t.Fatal("merge modified an input stream")
				}
			}
		})
	}
}

// 对照独立的全量排序结果，验证随机分路、重复索引和多页衔接。
func TestMergeFollowingStreamsMatchesFullSort(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	for trial := 0; trial < 100; trial++ {
		streams := make([][]FollowingCursor, 1+rng.Intn(12))
		all := make([]FollowingCursor, 80)
		for i := range all {
			entry := FollowingCursor{Time: at.Add(time.Duration(rng.Intn(10)) * time.Microsecond), ID: uint(i + 1)}
			all[i] = entry
			for copies := 1 + rng.Intn(3); copies > 0; copies-- {
				source := rng.Intn(len(streams))
				streams[source] = append(streams[source], entry)
			}
		}
		// Oracle directly expresses the SQL order, independently of heap comparison.
		newer := func(a, b FollowingCursor) bool {
			if a.Time.Equal(b.Time) {
				return a.ID > b.ID
			}
			return a.Time.After(b.Time)
		}
		sort.Slice(all, func(i, j int) bool { return newer(all[i], all[j]) })
		for _, stream := range streams {
			sort.Slice(stream, func(i, j int) bool { return newer(stream[i], stream[j]) })
		}
		limit := 1 + rng.Intn(15)
		got := make([]FollowingCursor, 0)
		var cursor *FollowingCursor
		for page := 0; page < len(all)+1; page++ {
			candidates := mergeFollowingStreams(streams, cursor, limit+1)
			hasMore := len(candidates) > limit
			if hasMore {
				candidates = candidates[:limit]
			}
			got = append(got, candidates...)
			if !hasMore {
				break
			}
			last := candidates[len(candidates)-1]
			cursor = &last
		}
		if !reflect.DeepEqual(got, all) {
			t.Fatalf("trial %d, limit %d: paged merge differs from full sort", trial, limit)
		}
	}
}
