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
