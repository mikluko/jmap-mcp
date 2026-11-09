package server

import (
	"slices"
	"testing"

	"sarak.sh/mikluko/jmap"
	"sarak.sh/mikluko/jmap/mail"
	"sarak.sh/mikluko/jmap/mail/email"
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

func TestAsReply(t *testing.T) {
	orig := &email.Email{
		MessageID:  []string{"m2@sarak.sh"},
		References: []string{"m0@sarak.sh", "m1@sarak.sh"},
		From:       []*mail.Address{{Name: "Mikhail", Email: "mikluko@sarak.sh"}},
		Subject:    "Introduction",
	}

	t.Run("threads under the original", func(t *testing.T) {
		draft := &email.Email{}
		asReply(draft, orig)
		if !slices.Equal(draft.InReplyTo, []string{"m2@sarak.sh"}) {
			t.Fatalf("InReplyTo is %v", draft.InReplyTo)
		}
		if !slices.Equal(draft.References, []string{"m0@sarak.sh", "m1@sarak.sh", "m2@sarak.sh"}) {
			t.Fatalf("References is %v", draft.References)
		}
	})

	t.Run("references fall back to in-reply-to", func(t *testing.T) {
		draft := &email.Email{}
		asReply(draft, &email.Email{MessageID: []string{"m2@sarak.sh"}, InReplyTo: []string{"m1@sarak.sh"}})
		if !slices.Equal(draft.References, []string{"m1@sarak.sh", "m2@sarak.sh"}) {
			t.Fatalf("References is %v", draft.References)
		}
	})

	t.Run("fills subject and recipient", func(t *testing.T) {
		draft := &email.Email{}
		asReply(draft, orig)
		if draft.Subject != "Re: Introduction" {
			t.Fatalf("Subject is %q", draft.Subject)
		}
		if len(draft.To) != 1 || draft.To[0].Email != "mikluko@sarak.sh" {
			t.Fatalf("To is %v", draft.To)
		}
	})

	t.Run("one Re prefix", func(t *testing.T) {
		draft := &email.Email{}
		asReply(draft, &email.Email{Subject: "RE: Introduction"})
		if draft.Subject != "RE: Introduction" {
			t.Fatalf("Subject is %q", draft.Subject)
		}
	})

	t.Run("reply-to wins over from", func(t *testing.T) {
		draft := &email.Email{}
		asReply(draft, &email.Email{From: orig.From, ReplyTo: []*mail.Address{{Email: "list@sarak.sh"}}})
		if len(draft.To) != 1 || draft.To[0].Email != "list@sarak.sh" {
			t.Fatalf("To is %v", draft.To)
		}
	})

	t.Run("keeps what the caller set", func(t *testing.T) {
		draft := &email.Email{Subject: "Other", To: []*mail.Address{{Email: "erg@sarak.sh"}}}
		asReply(draft, orig)
		if draft.Subject != "Other" || draft.To[0].Email != "erg@sarak.sh" {
			t.Fatalf("got %q to %v", draft.Subject, draft.To)
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

func TestSigned(t *testing.T) {
	for _, tc := range []struct {
		name, body, sig, want string
	}{
		{"no signature", "Hello.\n", "", "Hello.\n"},
		{"separator added", "Hello.", "— Mayor", "Hello.\n\n-- \n— Mayor"},
		{"trailing newlines folded", "Hello.\n\n\n", "— Mayor", "Hello.\n\n-- \n— Mayor"},
		{"separator kept", "Hello.", "-- \n— Mayor", "Hello.\n\n-- \n— Mayor"},
		{"already signed", "Hello.\n\n-- \n— Mayor\n", "— Mayor", "Hello.\n\n-- \n— Mayor\n"},
		{"empty body", "", "— Mayor", "-- \n— Mayor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := signed(tc.body, tc.sig); got != tc.want {
				t.Fatalf("signed(%q, %q) = %q, want %q", tc.body, tc.sig, got, tc.want)
			}
		})
	}
}

func TestNewDraftSignature(t *testing.T) {
	sender := &identity.Identity{ID: "b", Name: "Mayor of Saraksh", Email: "saraksh@cities.sarak.sh", TextSignature: "— Mayor, city Saraksh (Gas City)"}
	draft := newDraft(EmailCreateInput{Subject: "s", Body: "Hello."}, jmap.ID("drafts"), sender)

	want := "Hello.\n\n-- \n— Mayor, city Saraksh (Gas City)"
	if got := draft.BodyValues["body"].Value; got != want {
		t.Fatalf("body is %q, want %q", got, want)
	}
}
