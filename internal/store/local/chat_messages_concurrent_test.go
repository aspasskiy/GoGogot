package local

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/store"
)

func newConcurrentChatTestStore(t *testing.T) (*LocalStore, *store.Chat) {
	t.Helper()
	s := &LocalStore{dataDir: t.TempDir()}
	if err := s.ensureDirs(); err != nil {
		t.Fatalf("ensureDirs: %v", err)
	}
	ch := &store.Chat{
		ID:        "test-chat",
		StartedAt: time.Now(),
		Status:    "active",
	}
	ch.SetPersister(s)
	if err := s.SaveChat(ch); err != nil {
		t.Fatalf("SaveChat: %v", err)
	}
	return s, ch
}

// Regression test: AppendMessage must round-trip every concurrent write.
// The previous implementation issued two separate f.Write calls (payload +
// newline). POSIX O_APPEND only atomically advances the file offset for a
// single write within PIPE_BUF, so concurrent goroutines' second writes
// could interleave with another goroutine's first write — joining two
// JSON payloads onto one line (parse failure) and leaving an extra blank
// line behind. Both messages were silently dropped.
func TestAppendMessage_ConcurrentNoLoss(t *testing.T) {
	s, ch := newConcurrentChatTestStore(t)

	const N = 200
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			msg := domain.Turn{
				Role: "user",
				Content: []domain.ContentBlock{
					domain.TextBlock(fmt.Sprintf("msg-%d-with-padding-to-widen-the-race-window", i)),
				},
				Timestamp: time.Now(),
			}
			s.AppendMessage(ch, msg)
		}(i)
	}
	close(start)
	wg.Wait()

	fresh := &store.Chat{ID: ch.ID, StartedAt: ch.StartedAt}
	fresh.SetPersister(s)
	if err := s.LoadMessages(fresh); err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if got := len(fresh.Messages()); got != N {
		t.Errorf("expected %d messages after %d concurrent appends, got %d", N, N, got)
	}
}

func TestAppendMessage_SequentialRoundTrip(t *testing.T) {
	s, ch := newConcurrentChatTestStore(t)

	for i := 0; i < 5; i++ {
		s.AppendMessage(ch, domain.Turn{
			Role:      "user",
			Content:   []domain.ContentBlock{domain.TextBlock(fmt.Sprintf("seq-%d", i))},
			Timestamp: time.Now(),
		})
	}

	fresh := &store.Chat{ID: ch.ID, StartedAt: ch.StartedAt}
	fresh.SetPersister(s)
	if err := s.LoadMessages(fresh); err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	if got := len(fresh.Messages()); got != 5 {
		t.Errorf("expected 5 messages, got %d", got)
	}
}
