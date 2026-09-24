package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestSortedSetSnapshotAndOwnerLock(t *testing.T) {
	server := miniredis.RunT(t)
	client := NewClient(Options{Addr: server.Addr(), CommandTimeout: time.Second})
	defer client.Close()
	ctx := context.Background()
	if ok, err := client.SetNX(ctx, "ranking-lock", "owner-a", time.Minute); err != nil || !ok {
		t.Fatalf("acquire lock: ok=%v err=%v", ok, err)
	}
	if ok, err := client.SetNX(ctx, "ranking-lock", "owner-b", time.Minute); err != nil || ok {
		t.Fatalf("second owner acquired lock: ok=%v err=%v", ok, err)
	}
	if err := client.ZAdd(ctx, "ranking-building", ZMember{Score: 10, Member: "high"}, ZMember{Score: 10, Member: "low"}); err != nil {
		t.Fatalf("zadd: %v", err)
	}
	if err := client.Rename(ctx, "ranking-building", "ranking-active"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	items, err := client.ZRevRangeWithScores(ctx, "ranking-active", 0, -1)
	if err != nil || len(items) != 2 || items[0].Score != 10 {
		t.Fatalf("snapshot=%v err=%v", items, err)
	}
	if _, err := client.Eval(ctx, `if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`, []string{"ranking-lock"}, "owner-b"); err != nil {
		t.Fatalf("wrong owner unlock: %v", err)
	}
	if got, err := client.Get(ctx, "ranking-lock"); err != nil || got != "owner-a" {
		t.Fatalf("wrong owner changed lock: value=%q err=%v", got, err)
	}
}
