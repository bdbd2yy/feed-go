package redis

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestFollowingIndexOrderingAndBounds(t *testing.T) {
	mr := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer raw.Close()
	c := NewClient(raw, "")
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)
	entries := []string{FollowingMember(at, 9), FollowingMember(at, 10), FollowingMember(at.Add(time.Microsecond), 1)}
	for _, member := range entries {
		if err := c.AppendFollowingIndex(ctx, "outbox", member, 2, time.Hour, false); err != nil {
			t.Fatal(err)
		}
	}
	got, err := raw.ZRevRange(ctx, "outbox", 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{entries[2], entries[1]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	mr.FastForward(time.Hour)
	if mr.Exists("outbox") {
		t.Fatal("index did not expire")
	}
	if err := c.AppendFollowingIndex(ctx, "outbox", entries[0], 2, time.Hour, true); err != nil {
		t.Fatal(err)
	}
	if mr.Exists("outbox") {
		t.Fatal("incremental fanout recreated expired inbox")
	}
	if err := c.AppendFollowingIndex(ctx, "outbox", entries[0], 0, time.Hour, false); err == nil {
		t.Fatal("zero capacity accepted")
	}
}

func TestInboxRebuildPreservesConcurrentFanout(t *testing.T) {
	mr := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer raw.Close()
	c := NewClient(raw, "")
	ctx := context.Background()
	key := "inbox"
	token, ok, err := c.BeginInboxRebuild(ctx, key, time.Second)
	if err != nil || !ok {
		t.Fatalf("begin: %v %v", ok, err)
	}
	if _, ok, err := c.BeginInboxRebuild(ctx, key, time.Second); err != nil || ok {
		t.Fatalf("duplicate rebuild: %v %v", ok, err)
	}
	// 没有正式 inbox 时，增量也必须进入临时区。
	if err := c.AppendFollowingIndex(ctx, key, "D", 3, time.Hour, true); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.FinishInboxRebuild(ctx, key, token, []string{"A", "B", "C"}, 3, time.Hour); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}
	got, err := raw.ZRevRange(ctx, key, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"D", "C", "B"}) {
		t.Fatalf("concurrent index lost: %v", got)
	}
	if ready, err := c.TouchInbox(ctx, key, time.Hour); err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if err := c.AppendFollowingIndex(ctx, key, "E", 3, time.Hour, true); err != nil {
		t.Fatal(err)
	}
	if ready, _ := c.TouchInbox(ctx, key, time.Hour); ready {
		t.Fatal("incremental write must invalidate provisional coverage marker")
	}
}

func TestInboxRebuildFencesExpiredTask(t *testing.T) {
	mr := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer raw.Close()
	c := NewClient(raw, "")
	ctx := context.Background()
	key := "inbox"
	old, _, _ := c.BeginInboxRebuild(ctx, key, time.Second)
	_ = c.AppendFollowingIndex(ctx, key, "old", 3, time.Hour, true)
	mr.FastForward(2 * time.Second)
	token, ok, err := c.BeginInboxRebuild(ctx, key, time.Second)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if ok, err := c.FinishInboxRebuild(ctx, key, old, []string{"stale"}, 3, time.Hour); err != nil || ok {
		t.Fatal("expired task published")
	}
	if err := c.AbortInboxRebuild(ctx, key, old); err != nil {
		t.Fatal(err)
	}
	if ok, err := c.FinishInboxRebuild(ctx, key, token, nil, 3, time.Hour); err != nil || !ok {
		t.Fatal("old cleanup deleted new task")
	}
	if ready, err := c.TouchInbox(ctx, key, time.Hour); err != nil || !ready {
		t.Fatal("empty snapshot must be representable")
	}
	if mr.Exists(key) {
		t.Fatal("empty snapshot created data key")
	}
}
