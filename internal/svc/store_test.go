package svc

import (
	"testing"

	"a2a-platform/internal/model"
)

func TestAppend_SavesContextAndDirection(t *testing.T) {
	db := setupRegistryTestDB(t)

	store := NewMessageStore(db)

	ctxId := "ctx-test-123"
	msg := &model.Message{
		TaskId:    "task-1",
		ContextId: &ctxId,
		Role:      "user",
		Content:   "hello",
	}
	sender := "host"
	recipient := "assistant"
	msg.SenderAgent = &sender
	msg.RecipientAgent = &recipient

	err := store.Append(msg)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Query by context - should find the message but currently won't
	msgs, err := store.GetByContext(ctxId)
	if err != nil {
		t.Fatalf("GetByContext failed: %v", err)
	}

	if len(msgs) == 0 {
		t.Logf("BUG CONFIRMED: Message with context_id=%s was not found by GetByContext", ctxId)
		t.Errorf("Expected to find message by context_id, got 0 messages")
	} else if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	} else if msgs[0].Content != "hello" {
		t.Errorf("Expected content 'hello', got %q", msgs[0].Content)
	} else if msgs[0].SenderAgent == nil || *msgs[0].SenderAgent != "host" {
		t.Errorf("Expected sender_agent host, got %v", msgs[0].SenderAgent)
	} else if msgs[0].RecipientAgent == nil || *msgs[0].RecipientAgent != "assistant" {
		t.Errorf("Expected recipient_agent assistant, got %v", msgs[0].RecipientAgent)
	}
}

// TestAppendWithContext_SavesContextId verifies that AppendWithContext correctly saves context_id.
func TestAppendWithContext_SavesContextId(t *testing.T) {
	db := setupRegistryTestDB(t)

	store := NewMessageStore(db)

	ctxId := "ctx-test-456"
	msg := &model.Message{
		TaskId:    "task-2",
		ContextId: &ctxId,
		Role:      "agent",
		Content:   "response",
	}

	err := store.AppendWithContext(msg)
	if err != nil {
		t.Fatalf("AppendWithContext failed: %v", err)
	}

	msgs, err := store.GetByContext(ctxId)
	if err != nil {
		t.Fatalf("GetByContext failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	} else if msgs[0].Content != "response" {
		t.Errorf("Expected content 'response', got %q", msgs[0].Content)
	}
}

// TestGetByContext_ReturnsMessagesInOrder verifies message ordering by timestamp.
func TestGetByContext_ReturnsMessagesInOrder(t *testing.T) {
	db := setupRegistryTestDB(t)

	store := NewMessageStore(db)
	ctxId := "ctx-ordered"

	for i, content := range []string{"first", "second", "third"} {
		msg := &model.Message{
			TaskId:    "task-" + string(rune('a'+i)),
			ContextId: &ctxId,
			Role:      "user",
			Content:   content,
		}
		if err := store.AppendWithContext(msg); err != nil {
			t.Fatalf("AppendWithContext failed: %v", err)
		}
	}

	msgs, err := store.GetByContext(ctxId)
	if err != nil {
		t.Fatalf("GetByContext failed: %v", err)
	}

	if len(msgs) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(msgs))
	}

	expected := []string{"first", "second", "third"}
	for i, m := range msgs {
		if m.Content != expected[i] {
			t.Errorf("Message %d: expected %q, got %q", i, expected[i], m.Content)
		}
	}
}
