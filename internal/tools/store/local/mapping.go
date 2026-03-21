package local

import (
	"fmt"
	"os"
	"strings"
)

func (s *LocalStore) GetActiveChatID() (string, error) {
	chatID, err := s.loadActiveChatID()
	if err != nil {
		return "", err
	}
	if chatID == "" {
		return "", fmt.Errorf("no active chat")
	}
	return chatID, nil
}

func (s *LocalStore) SetActiveChatID(chatID string) error {
	return os.WriteFile(s.activeChatPath(), []byte(chatID+"\n"), 0o644)
}

func (s *LocalStore) loadActiveChatID() (string, error) {
	data, err := os.ReadFile(s.activeChatPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
