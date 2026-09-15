package agent

import (
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

func TestWithSessionIDResumesExistingSession(t *testing.T) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{ID: "resume-me", Title: "history"})
	if err != nil {
		t.Fatal(err)
	}
	agt, err := New(WithModel(fakeOwnerModel{}), WithSession(store), WithSessionID(info.ID))
	if err != nil {
		t.Fatalf("New with existing session: %v", err)
	}
	t.Cleanup(func() { _ = agt.Close() })
	got, ok := agt.(SessionIDProvider)
	if !ok || got.SessionID() != info.ID {
		t.Fatalf("agent did not bind selected session: provider=%v id=%q", ok, got.SessionID())
	}
	if _, err := New(WithModel(fakeOwnerModel{}), WithSession(store), WithSessionID("missing")); err == nil {
		t.Fatal("missing selected session must fail instead of silently creating a new one")
	}
}
