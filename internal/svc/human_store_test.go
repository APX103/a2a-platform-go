package svc

import (
	"testing"
	"time"
)

func TestHumanUserStoreCreateAndDuplicate(t *testing.T) {
	db := setupRegistryTestDB(t)
	store := NewHumanUserStore(db)

	user, err := store.Create("Alice_1", "Alice")
	if err != nil {
		t.Fatalf("create human: %v", err)
	}
	if user.ID == "" {
		t.Fatal("human id was empty")
	}
	if user.Handle != "alice_1" {
		t.Fatalf("handle = %q, want alice_1", user.Handle)
	}
	if _, err := store.Create("alice_1", "Alice Again"); err == nil {
		t.Fatal("expected duplicate handle error")
	}
	if loaded, err := store.GetByHandle("alice_1"); err != nil {
		t.Fatalf("get by handle: %v", err)
	} else if loaded == nil || loaded.ID != user.ID {
		t.Fatalf("loaded = %#v, want %s", loaded, user.ID)
	}
}

func TestHumanSessionStoreCreateAndLookup(t *testing.T) {
	db := setupRegistryTestDB(t)
	users := NewHumanUserStore(db)
	sessions := NewHumanSessionStore(db)

	user, err := users.Create("bob", "Bob")
	if err != nil {
		t.Fatalf("create human: %v", err)
	}
	session, token, err := sessions.Create(user.ID, 0)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID == 0 || token == "" {
		t.Fatalf("session/token = %#v/%q", session, token)
	}
	lookedUp, err := sessions.GetByToken(token)
	if err != nil {
		t.Fatalf("lookup session: %v", err)
	}
	if !HumanSessionUsable(lookedUp, session.CreatedAt) {
		t.Fatalf("session should be usable: %#v", lookedUp)
	}
	if lookedUp.HumanID != user.ID {
		t.Fatalf("human id = %q, want %q", lookedUp.HumanID, user.ID)
	}
	if err := sessions.Revoke(token); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	revoked, err := sessions.GetByToken(token)
	if err != nil {
		t.Fatalf("lookup revoked session: %v", err)
	}
	if HumanSessionUsable(revoked, session.CreatedAt) {
		t.Fatal("revoked session should not be usable")
	}
}

func TestHumanUserStoreListPresence(t *testing.T) {
	db := setupRegistryTestDB(t)
	users := NewHumanUserStore(db)
	sessions := NewHumanSessionStore(db)

	onlineUser, err := users.Create("online-human", "Online Human")
	if err != nil {
		t.Fatalf("create online human: %v", err)
	}
	if _, _, err := sessions.Create(onlineUser.ID, 0); err != nil {
		t.Fatalf("create online session: %v", err)
	}
	if err := users.TouchLastSeen(onlineUser.ID); err != nil {
		t.Fatalf("touch last seen: %v", err)
	}
	if _, err := users.Create("offline-human", "Offline Human"); err != nil {
		t.Fatalf("create offline human: %v", err)
	}

	humans, err := users.ListPresence(time.Now(), 90*time.Second)
	if err != nil {
		t.Fatalf("list presence: %v", err)
	}
	byHandle := map[string]bool{}
	for _, human := range humans {
		byHandle[human.Handle] = human.Online
		if human.Handle == "online-human" && human.ActiveSessions != 1 {
			t.Fatalf("active sessions = %d, want 1", human.ActiveSessions)
		}
	}
	if !byHandle["online-human"] {
		t.Fatal("online human should be marked online")
	}
	if byHandle["offline-human"] {
		t.Fatal("offline human should not be marked online")
	}
}

func TestHumanUserStoreUpdateAndDelete(t *testing.T) {
	db := setupRegistryTestDB(t)
	users := NewHumanUserStore(db)
	sessions := NewHumanSessionStore(db)

	user, err := users.Create("editable-human", "Editable")
	if err != nil {
		t.Fatalf("create human: %v", err)
	}
	if _, _, err := sessions.Create(user.ID, 0); err != nil {
		t.Fatalf("create session: %v", err)
	}
	updated, err := users.Update(user.ID, "renamed-human", "Renamed")
	if err != nil {
		t.Fatalf("update human: %v", err)
	}
	if updated == nil || updated.Handle != "renamed-human" || updated.DisplayName != "Renamed" {
		t.Fatalf("updated human = %#v", updated)
	}
	if loaded, err := users.GetByHandle("renamed-human"); err != nil {
		t.Fatalf("get renamed human: %v", err)
	} else if loaded == nil || loaded.ID != user.ID {
		t.Fatalf("loaded renamed human = %#v", loaded)
	}

	deleted, err := users.Delete(user.ID)
	if err != nil {
		t.Fatalf("delete human: %v", err)
	}
	if !deleted {
		t.Fatal("delete should report true")
	}
	if loaded, err := users.Get(user.ID); err != nil {
		t.Fatalf("get deleted human: %v", err)
	} else if loaded != nil {
		t.Fatalf("deleted human still loaded: %#v", loaded)
	}
	var sessionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM human_sessions WHERE human_id = ?`, user.ID).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("session count = %d, want 0", sessionCount)
	}
}
