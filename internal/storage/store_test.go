package storage

import (
	"testing"
	"time"
)

func TestDeleteRemovesMessage(t *testing.T) {
	store := New(0, 0)
	store.Add(" Inbox ", Message{ID: "1"})
	store.Add("inbox", Message{ID: "2"})

	if deleted := store.Delete("INBOX", "1"); !deleted {
		t.Fatalf("expected delete to return true")
	}

	messages := store.List("inbox")
	if len(messages) != 1 {
		t.Fatalf("expected 1 message remaining, got %d", len(messages))
	}
	if messages[0].ID != "2" {
		t.Fatalf("expected remaining message 2, got %s", messages[0].ID)
	}
}

func TestClearRemovesAllMessagesInMailbox(t *testing.T) {
	store := New(0, 0)
	store.Add("demo", Message{ID: "1"})
	store.Add("demo", Message{ID: "2"})
	store.Add("other", Message{ID: "3"})

	count := store.Clear(" Demo ")
	if count != 2 {
		t.Fatalf("expected clear count 2, got %d", count)
	}

	if got := len(store.List("demo")); got != 0 {
		t.Fatalf("expected demo mailbox to be empty, got %d", got)
	}
	if got := len(store.List("other")); got != 1 {
		t.Fatalf("expected other mailbox to keep 1 message, got %d", got)
	}
}

func TestClearPrunesExpiredBeforeCount(t *testing.T) {
	store := New(0, time.Hour)
	store.Add("demo", Message{ID: "fresh", ReceivedAt: time.Now().UTC()})
	store.Add("demo", Message{ID: "old", ReceivedAt: time.Now().UTC().Add(-2 * time.Hour)})

	count := store.Clear("demo")
	if count != 1 {
		t.Fatalf("expected clear count to ignore expired messages, got %d", count)
	}
}
