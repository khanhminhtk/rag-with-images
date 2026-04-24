package orchestrator

import (
	"sync"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports/orchestrator"
)

type InMemoryChatMessageManager struct {
	mu      sync.RWMutex
	Message []orchestrator.ChatMessage
}

func NewInMemoryChatMessageManager() *InMemoryChatMessageManager {
	return &InMemoryChatMessageManager{
		Message: []orchestrator.ChatMessage{},
	}
}

func (i *InMemoryChatMessageManager) AddMessage(message orchestrator.ChatMessage) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.Message = append(i.Message, message)
	return nil
}

func (i *InMemoryChatMessageManager) GetChatHistory() ([]orchestrator.ChatMessage, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	history := make([]orchestrator.ChatMessage, len(i.Message))
	copy(history, i.Message)
	return history, nil
}

func (i *InMemoryChatMessageManager) ClearChatHistory() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.Message = []orchestrator.ChatMessage{}
	return nil
}

type inMemorySessionEntry struct {
	Data      orchestrator.SessionData
	ExpiresAt time.Time
}

type InMemorySessionStore struct {
	mu                sync.RWMutex
	Sessions          map[string]inMemorySessionEntry
	sessionTTL        time.Duration
	cleanupTicker     *time.Ticker
	stopCleanupCh     chan struct{}
	closeOnce         sync.Once
	onSessionReleased func(sessionID string)
}

func (s *InMemorySessionStore) CreateSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.Sessions[sessionID] = inMemorySessionEntry{
		Data: orchestrator.SessionData{
			SessionID:   sessionID,
			ChatHistory: NewInMemoryChatMessageManager(),
			Metadata: orchestrator.Metadata{
				CreatedAt: now,
			},
		},
		ExpiresAt: now.Add(s.sessionTTL),
	}
	return nil
}

func (s *InMemorySessionStore) GetSession(sessionID string) (orchestrator.SessionData, error) {
	s.mu.Lock()

	entry, exists := s.Sessions[sessionID]
	if !exists {
		s.mu.Unlock()
		return orchestrator.SessionData{}, nil
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(s.Sessions, sessionID)
		callback := s.onSessionReleased
		s.mu.Unlock()
		if callback != nil {
			callback(sessionID)
		}
		return orchestrator.SessionData{}, nil
	}

	entry.ExpiresAt = time.Now().Add(s.sessionTTL)
	s.Sessions[sessionID] = entry
	s.mu.Unlock()

	return entry.Data, nil
}

func (s *InMemorySessionStore) DeleteSession(sessionID string) error {
	s.mu.Lock()
	_, exists := s.Sessions[sessionID]
	delete(s.Sessions, sessionID)
	callback := s.onSessionReleased
	s.mu.Unlock()
	if exists && callback != nil {
		callback(sessionID)
	}
	return nil
}

func (s *InMemorySessionStore) SessionExists(sessionID string) (bool, error) {
	s.mu.RLock()
	entry, exists := s.Sessions[sessionID]
	s.mu.RUnlock()
	if !exists {
		return false, nil
	}
	if time.Now().After(entry.ExpiresAt) {
		s.mu.Lock()
		shouldRelease := false
		if current, ok := s.Sessions[sessionID]; ok && time.Now().After(current.ExpiresAt) {
			delete(s.Sessions, sessionID)
			shouldRelease = true
		}
		callback := s.onSessionReleased
		s.mu.Unlock()
		if shouldRelease && callback != nil {
			callback(sessionID)
		}
		return false, nil
	}
	return exists, nil
}

func (s *InMemorySessionStore) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.stopCleanupCh)
		s.mu.Lock()
		sessionIDs := make([]string, 0, len(s.Sessions))
		for sessionID := range s.Sessions {
			sessionIDs = append(sessionIDs, sessionID)
			delete(s.Sessions, sessionID)
		}
		callback := s.onSessionReleased
		s.mu.Unlock()
		if callback != nil {
			for _, sessionID := range sessionIDs {
				callback(sessionID)
			}
		}
	})
}

func (s *InMemorySessionStore) startCleanupLoop() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.cleanupExpiredSessions(time.Now())
		case <-s.stopCleanupCh:
			s.cleanupTicker.Stop()
			return
		}
	}
}

func (s *InMemorySessionStore) cleanupExpiredSessions(now time.Time) {
	s.mu.Lock()
	expiredSessionIDs := make([]string, 0)
	for sessionID, entry := range s.Sessions {
		if now.After(entry.ExpiresAt) {
			delete(s.Sessions, sessionID)
			expiredSessionIDs = append(expiredSessionIDs, sessionID)
		}
	}
	callback := s.onSessionReleased
	s.mu.Unlock()
	if callback != nil {
		for _, sessionID := range expiredSessionIDs {
			callback(sessionID)
		}
	}
}

func (s *InMemorySessionStore) SetOnSessionReleased(callback func(sessionID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSessionReleased = callback
}

func NewInMemorySessionStore(sessionTTL time.Duration) *InMemorySessionStore {
	if sessionTTL <= 0 {
		sessionTTL = 30 * time.Minute
	}

	cleanupInterval := sessionTTL / 2
	if cleanupInterval < 10*time.Second {
		cleanupInterval = 10 * time.Second
	}

	store := &InMemorySessionStore{
		Sessions:      make(map[string]inMemorySessionEntry),
		sessionTTL:    sessionTTL,
		cleanupTicker: time.NewTicker(cleanupInterval),
		stopCleanupCh: make(chan struct{}),
	}
	go store.startCleanupLoop()
	return store
}
