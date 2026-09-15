package main

import (
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

func TestProjectSessionMessagesUsesSelectedSessionOnly(t *testing.T) {
	store, err := session.NewJSONLStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Create(session.CreateOpts{ID: "history-one", Title: "history"})
	if err != nil {
		t.Fatal(err)
	}
	appendData := func(typ session.EventType, data any) {
		raw, encodeErr := session.EncodeData(data)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if appendErr := store.AppendEvent(session.Event{SessionID: info.ID, Type: typ, Data: raw}); appendErr != nil {
			t.Fatal(appendErr)
		}
	}
	appendData(session.EventPrompted, &session.PromptedData{Content: "SESSION-ONE-UNIQUE"})
	appendData(session.EventTextDelta, &session.TextDeltaData{Delta: "历史答复"})
	appendData(session.EventTextEnded, nil)

	got := projectSessionMessages(store, info.ID)
	if len(got) != 2 {
		t.Fatalf("projected message count = %d, want 2: %#v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Content != "SESSION-ONE-UNIQUE" {
		t.Fatalf("user history = %#v", got[0])
	}
	if got[1].Role != "assistant" || got[1].Content != "历史答复" {
		t.Fatalf("assistant history = %#v", got[1])
	}
	if other := projectSessionMessages(store, "missing"); other != nil {
		t.Fatalf("missing session should project no history: %#v", other)
	}
}
