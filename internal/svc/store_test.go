package svc

import (
	"database/sql"
	"path/filepath"
	"testing"

	"a2a-platform/internal/model"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		context_id TEXT,
		role TEXT NOT NULL,
		content TEXT,
		reasoning_content TEXT,
		tool_calls TEXT,
		tool_call_id TEXT,
		thinking_blocks TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_messages_task_id ON messages(task_id);
	CREATE INDEX IF NOT EXISTS idx_messages_context_id ON messages(context_id);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

// TestAppend_DoesNotSaveContextId verifies that Append() ignores context_id,
// which is a bug causing messages to be lost when loading history by context.
func TestAppend_DoesNotSaveContextId(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewMessageStore(db)

	ctxId := "ctx-test-123"
	msg := &model.Message{
		TaskId:    "task-1",
		ContextId: &ctxId,
		Role:      "user",
		Content:   "hello",
	}

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
	}
}

// TestAppendWithContext_SavesContextId verifies that AppendWithContext correctly saves context_id.
func TestAppendWithContext_SavesContextId(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

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
	db := setupTestDB(t)
	defer db.Close()

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
