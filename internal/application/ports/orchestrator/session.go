package orchestrator

import "time"

type ChatMessage struct {
	Role string
	Content string
}

type ChatMessageManager interface {
	AddMessage(message ChatMessage) error
	GetChatHistory() ([]ChatMessage, error)
	ClearChatHistory() error
}

type Metadata struct {
	CreatedAt time.Time
}

type SessionData struct {
	SessionID   string
	ChatHistory ChatMessageManager
	Metadata    Metadata
}

type SessionStore interface {
	CreateSession(sessionID string) error
	GetSession(sessionID string) (SessionData, error)
	DeleteSession(sessionID string) error
	SessionExists(sessionID string) (bool, error)
}