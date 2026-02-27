package storage

import (
	"testing"
	"time"
)

func TestSubscribeReceivesNewMessageSummary(t *testing.T) {
	store := New(100, 0)
	events, unsubscribe := store.Subscribe("Inbox")
	defer unsubscribe()

	store.Add("Inbox", Message{
		ID:      "msg-1",
		From:    "alice@example.com",
		Subject: "hello",
	})

	select {
	case event := <-events:
		if event.ID != "msg-1" {
			t.Fatalf("unexpected event id: %s", event.ID)
		}
		if event.Subject != "hello" {
			t.Fatalf("unexpected subject: %s", event.Subject)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mailbox event")
	}
}

func TestUnsubscribeStopsMailboxDelivery(t *testing.T) {
	store := New(100, 0)
	events, unsubscribe := store.Subscribe("Inbox")

	unsubscribe()
	store.Add("Inbox", Message{ID: "msg-2", Subject: "after-unsubscribe"})

	select {
	case event := <-events:
		t.Fatalf("unexpected event after unsubscribe: %+v", event)
	case <-time.After(200 * time.Millisecond):
	}
}
