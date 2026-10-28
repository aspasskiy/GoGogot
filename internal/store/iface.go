package store

import (
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
)

// Store defines the persistence contract for all application data.
// Implementations may use a local filesystem, S3, or any other backend.
type Store interface {
	ChatPersister

	// Chats
	NewChat() *Chat
	LoadChat(id string) (*Chat, error)
	ListChats() ([]domain.ChatInfo, error)

	// Active chat mapping
	GetActiveChatID() (string, error)
	SetActiveChatID(id string) error

	// Memory
	ListMemory() ([]MemoryFile, error)
	ReadMemory(filename string) (string, error)
	WriteMemory(filename, content string) error
	DeleteMemory(filename string) error

	// Identity
	ReadSoul() string
	WriteSoul(content string) error
	ReadUser() string
	WriteUser(content string) error
	LoadTimezone() *time.Location

	// Skills
	SkillsDir() string
	LoadSkills() ([]domain.Skill, error)
	CreateSkill(name, description, body string) (string, error)
	UpdateSkill(name, content string) error
	DeleteSkill(name string) error
	ReadSkill(name string) (string, error)
}

// ChatPersister is the subset of Store that Chat delegates its I/O to.
// Concrete store implementations satisfy this automatically by implementing Store.
type ChatPersister interface {
	SaveChat(ch *Chat) error
	LoadMessages(ch *Chat) error
	AppendMessage(ch *Chat, msg domain.Turn)
	ReplaceMessages(ch *Chat, msgs []domain.Turn) error
	TextMessages(ch *Chat) ([]Message, error)
	HasMessages(ch *Chat) bool
}
