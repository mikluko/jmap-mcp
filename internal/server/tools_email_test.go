package server

import (
	"testing"

	"sarak.sh/mikluko/jmap"
	"sarak.sh/mikluko/jmap/mail/identity"
)

func TestSenderIdentity(t *testing.T) {
	agent := &identity.Identity{ID: "i1", Name: "erg, the agent", Email: "erg@sarak.sh"}
	alias := &identity.Identity{ID: "i2", Email: "erg+task@sarak.sh"}

	t.Run("first when id is empty", func(t *testing.T) {
		got, err := senderIdentity([]*identity.Identity{agent, alias}, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != agent {
			t.Fatalf("got %v, want %v", got, agent)
		}
	})

	t.Run("by id", func(t *testing.T) {
		got, err := senderIdentity([]*identity.Identity{agent, alias}, "i2")
		if err != nil {
			t.Fatal(err)
		}
		if got != alias {
			t.Fatalf("got %v, want %v", got, alias)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		if _, err := senderIdentity([]*identity.Identity{agent}, "nope"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("no identities", func(t *testing.T) {
		if _, err := senderIdentity(nil, ""); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNewDraftFrom(t *testing.T) {
	sender := &identity.Identity{ID: "i1", Name: "erg, the agent", Email: "erg@sarak.sh"}
	draft := newDraft(EmailCreateInput{To: []string{"mikluko@sarak.sh"}, Subject: "s", Body: "b"}, jmap.ID("drafts"), sender)

	if len(draft.From) != 1 {
		t.Fatalf("got %d From addresses, want 1", len(draft.From))
	}
	if got := draft.From[0]; got.Name != sender.Name || got.Email != sender.Email {
		t.Fatalf("From is %q <%s>, want %q <%s>", got.Name, got.Email, sender.Name, sender.Email)
	}
	if !draft.MailboxIDs["drafts"] {
		t.Fatalf("draft is not in the Drafts mailbox: %v", draft.MailboxIDs)
	}
}
