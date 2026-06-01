package session

import (
	"container/list"
	"sync"
	"time"
)

// Entry holds the stored session data for a response_id.
type Entry struct {
	ResponseID string
	Messages   string
	Reasoning  string
	createdAt  time.Time
	expiresAt  time.Time
}

// Store is a thread-safe LRU cache with TTL for session entries.
type Store struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	lru      *list.List
}

// New creates a new Store with the given capacity and TTL.
func New(capacity int, ttl time.Duration) *Store {
	return &Store{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Put stores an entry for the given response_id.
func (s *Store) Put(responseID, messages, reasoning string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	entry := &Entry{
		ResponseID: responseID,
		Messages:   messages,
		Reasoning:  reasoning,
		createdAt:  now,
		expiresAt:  now.Add(s.ttl),
	}

	// Update existing
	if el, ok := s.items[responseID]; ok {
		el.Value = entry
		s.lru.MoveToFront(el)
		return
	}

	// Evict expired
	s.evictExpired()

	// Evict LRU if at capacity
	for s.lru.Len() >= s.capacity {
		s.evictLRU()
	}

	el := s.lru.PushFront(entry)
	s.items[responseID] = el
}

// Get retrieves an entry by response_id.
func (s *Store) Get(responseID string) (*Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	el, ok := s.items[responseID]
	if !ok {
		return nil, false
	}

	entry := el.Value.(*Entry)
	if time.Now().After(entry.expiresAt) {
		s.lru.Remove(el)
		delete(s.items, responseID)
		return nil, false
	}

	s.lru.MoveToFront(el)
	return entry, true
}

// Len returns the current number of entries.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lru.Len()
}

func (s *Store) evictExpired() {
	now := time.Now()
	for el := s.lru.Back(); el != nil; {
		prev := el.Prev()
		entry := el.Value.(*Entry)
		if now.After(entry.expiresAt) {
			s.lru.Remove(el)
			delete(s.items, entry.ResponseID)
		}
		el = prev
	}
}

func (s *Store) evictLRU() {
	el := s.lru.Back()
	if el != nil {
		entry := el.Value.(*Entry)
		s.lru.Remove(el)
		delete(s.items, entry.ResponseID)
	}
}
