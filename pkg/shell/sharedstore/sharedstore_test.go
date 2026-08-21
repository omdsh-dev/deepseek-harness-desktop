package sharedstore

import (
	"testing"
)

func TestStoreBasics(t *testing.T) {
	s := New(t.TempDir())

	// 写 + 读
	s.Set("k1", "v1")
	if v, ok := s.Get("k1"); !ok || v != "v1" {
		t.Fatalf("读应为 v1: %q %v", v, ok)
	}
	// 覆盖写
	s.Set("k1", "v2")
	if v, _ := s.Get("k1"); v != "v2" {
		t.Fatalf("覆盖后应为 v2: %q", v)
	}
	// 读不存在的 key
	if _, ok := s.Get("nope"); ok {
		t.Fatal("不存在的 key 应返回 ok=false")
	}
	// 删除
	s.Remove("k1")
	if _, ok := s.Get("k1"); ok {
		t.Fatal("删除后应不存在")
	}
	// clear
	s.Set("a", "1")
	s.Set("b", "2")
	s.Clear()
	if _, ok := s.Get("a"); ok {
		t.Fatal("clear 后应不存在")
	}
	if _, ok := s.Get("b"); ok {
		t.Fatal("clear 后应不存在")
	}
	s.Flush() // 等落盘完成，避免异步写盘 goroutine 残留
}

func TestStorePersistAcrossRestart(t *testing.T) {
	home := t.TempDir()
	s := New(home)
	s.Set("dsh.sessions.current", `"sess-1"`)
	s.Flush() // 等落盘完成

	// 模拟重启：新 Store 读同一 home
	s2 := New(home)
	s2.Load()
	if v, ok := s2.Get("dsh.sessions.current"); !ok || v != `"sess-1"` {
		t.Fatalf("重启后应恢复状态: %q %v", v, ok)
	}
}

func TestStoreSnapshot(t *testing.T) {
	s := New(t.TempDir())
	s.Set("k1", "v1")
	s.Set("k2", "v2")
	s.Set("k1", "v1b") // 覆盖不改变顺序

	state, order := s.Snapshot()
	if len(state) != 2 || state["k1"] != "v1b" || state["k2"] != "v2" {
		t.Fatalf("快照状态不符: %v", state)
	}
	if len(order) != 2 || order[0] != "k1" || order[1] != "k2" {
		t.Fatalf("快照顺序不符: %v", order)
	}

	// 快照是副本：修改原 store 不影响已拿到的快照
	s.Set("k3", "v3")
	if _, ok := state["k3"]; ok {
		t.Fatal("快照应为副本，不受后续写入影响")
	}
	s.Flush()
}
