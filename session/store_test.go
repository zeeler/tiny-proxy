package session

import (
	"testing"
	"time"
)

func TestStorePutAndGet(t *testing.T) {
	s := New(10, time.Hour)
	s.Put("resp_1", `[{"role":"user","content":"hello"}]`, "thinking...")

	got, ok := s.Get("resp_1")
	if !ok {
		t.Fatal("expected entry to exist")
	}
	if got.Messages != `[{"role":"user","content":"hello"}]` {
		t.Errorf("messages = %q", got.Messages)
	}
	if got.Reasoning != "thinking..." {
		t.Errorf("reasoning = %q", got.Reasoning)
	}
	if got.ResponseID != "resp_1" {
		t.Errorf("ResponseID = %q", got.ResponseID)
	}
}

func TestStoreGetMissing(t *testing.T) {
	s := New(10, time.Hour)
	_, ok := s.Get("resp_nonexistent")
	if ok {
		t.Error("expected missing entry to return false")
	}
}

func TestStoreTTLExpiry(t *testing.T) {
	s := New(10, 10*time.Millisecond)
	s.Put("resp_1", "[]", "")
	time.Sleep(20 * time.Millisecond)
	_, ok := s.Get("resp_1")
	if ok {
		t.Error("expected expired entry to be gone")
	}
}

func TestStoreLRUEviction(t *testing.T) {
	s := New(3, time.Hour)
	s.Put("a", "[]", "")
	s.Put("b", "[]", "")
	s.Put("c", "[]", "")
	s.Get("a")           // make 'a' recently used
	s.Put("d", "[]", "") // should evict 'b' (LRU)

	if _, ok := s.Get("a"); !ok {
		t.Error("a should still exist")
	}
	if _, ok := s.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := s.Get("c"); !ok {
		t.Error("c should still exist")
	}
	if _, ok := s.Get("d"); !ok {
		t.Error("d should exist")
	}
}

func TestStoreLen(t *testing.T) {
	s := New(10, time.Hour)
	s.Put("a", "[]", "")
	s.Put("b", "[]", "")
	if s.Len() != 2 {
		t.Errorf("Len() = %d, want 2", s.Len())
	}
}
