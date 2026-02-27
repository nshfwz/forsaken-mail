package storage

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

type Message struct {
	ID         string              `json:"id"`
	Mailbox    string              `json:"mailbox"`
	To         string              `json:"to"`
	From       string              `json:"from"`
	Subject    string              `json:"subject"`
	Date       time.Time           `json:"date"`
	Text       string              `json:"text,omitempty"`
	HTML       string              `json:"html,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	ReceivedAt time.Time           `json:"received_at"`
}

type MessageSummary struct {
	ID         string    `json:"id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	Date       time.Time `json:"date"`
	HasHTML    bool      `json:"has_html"`
	Preview    string    `json:"preview"`
	ReceivedAt time.Time `json:"received_at"`
}

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

type Store struct {
	mu          sync.RWMutex
	byMailbox   map[string][]Message
	subscribers map[string]map[chan MessageSummary]struct{}
	maxMessages int
	ttl         time.Duration
}

func New(maxMessages int, ttl time.Duration) *Store {
	return &Store{
		byMailbox:   make(map[string][]Message),
		subscribers: make(map[string]map[chan MessageSummary]struct{}),
		maxMessages: maxMessages,
		ttl:         ttl,
	}
}

func (s *Store) Add(mailbox string, msg Message) {
	s.mu.Lock()

	now := time.Now().UTC()
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))

	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = now
	}
	if msg.Date.IsZero() {
		msg.Date = msg.ReceivedAt
	}
	if msg.Mailbox == "" {
		msg.Mailbox = mailbox
	}
	msg.Headers = cloneHeaders(msg.Headers)

	s.byMailbox[mailbox] = append(s.byMailbox[mailbox], msg)
	s.pruneMailboxLocked(mailbox, now)
	summary := msg.Summary()
	subs := s.collectSubscribersLocked(mailbox)
	s.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub <- summary:
		default:
		}
	}
}

func (s *Store) Subscribe(mailbox string) (<-chan MessageSummary, func()) {
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	ch := make(chan MessageSummary, 8)

	s.mu.Lock()
	if s.subscribers[mailbox] == nil {
		s.subscribers[mailbox] = make(map[chan MessageSummary]struct{})
	}
	s.subscribers[mailbox][ch] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		subs, ok := s.subscribers[mailbox]
		if !ok {
			return
		}
		if _, exists := subs[ch]; !exists {
			return
		}

		delete(subs, ch)
		if len(subs) == 0 {
			delete(s.subscribers, mailbox)
		}
	}

	return ch, unsubscribe
}

func (s *Store) List(mailbox string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	now := time.Now().UTC()
	s.pruneMailboxLocked(mailbox, now)

	messages := s.byMailbox[mailbox]
	out := make([]Message, len(messages))
	for i := range messages {
		out[i] = cloneMessage(messages[len(messages)-1-i])
	}
	return out
}

func (s *Store) Get(mailbox, id string) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	now := time.Now().UTC()
	s.pruneMailboxLocked(mailbox, now)

	messages := s.byMailbox[mailbox]
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ID == id {
			return cloneMessage(messages[i]), true
		}
	}
	return Message{}, false
}

func (s *Store) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	removed := 0
	for mailbox, messages := range s.byMailbox {
		before := len(messages)
		s.pruneMailboxLocked(mailbox, now)
		after := len(s.byMailbox[mailbox])
		if before > after {
			removed += before - after
		}
	}
	return removed
}

func (m Message) Summary() MessageSummary {
	previewSource := strings.TrimSpace(m.Text)
	if previewSource == "" {
		previewSource = strings.TrimSpace(htmlTagPattern.ReplaceAllString(m.HTML, " "))
	}
	previewSource = strings.Join(strings.Fields(previewSource), " ")
	previewRunes := []rune(previewSource)
	if len(previewRunes) > 120 {
		previewSource = string(previewRunes[:120]) + "..."
	}

	return MessageSummary{
		ID:         m.ID,
		From:       m.From,
		Subject:    m.Subject,
		Date:       m.Date,
		HasHTML:    strings.TrimSpace(m.HTML) != "",
		Preview:    previewSource,
		ReceivedAt: m.ReceivedAt,
	}
}

func (s *Store) pruneMailboxLocked(mailbox string, now time.Time) {
	messages, ok := s.byMailbox[mailbox]
	if !ok || len(messages) == 0 {
		delete(s.byMailbox, mailbox)
		return
	}

	if s.ttl > 0 {
		cutoff := now.Add(-s.ttl)
		filtered := make([]Message, 0, len(messages))
		for _, item := range messages {
			if !item.ReceivedAt.Before(cutoff) {
				filtered = append(filtered, item)
			}
		}
		messages = filtered
	}

	if s.maxMessages > 0 && len(messages) > s.maxMessages {
		messages = messages[len(messages)-s.maxMessages:]
	}

	if len(messages) == 0 {
		delete(s.byMailbox, mailbox)
		return
	}

	s.byMailbox[mailbox] = messages
}

func cloneMessage(msg Message) Message {
	out := msg
	out.Headers = cloneHeaders(msg.Headers)
	return out
}

func cloneHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return nil
	}

	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func (s *Store) collectSubscribersLocked(mailbox string) []chan MessageSummary {
	subs, ok := s.subscribers[mailbox]
	if !ok || len(subs) == 0 {
		return nil
	}

	out := make([]chan MessageSummary, 0, len(subs))
	for ch := range subs {
		out = append(out, ch)
	}
	return out
}
