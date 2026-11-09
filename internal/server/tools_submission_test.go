package server

import (
	"testing"

	"sarak.sh/mikluko/jmap/mail"
	"sarak.sh/mikluko/jmap/mail/identity"
)

func TestSubmissionIdentity(t *testing.T) {
	agent := &identity.Identity{ID: "i1", Email: "erg@sarak.sh"}
	alias := &identity.Identity{ID: "i2", Email: "erg+task@sarak.sh"}
	idents := []*identity.Identity{agent, alias}
	fromAlias := []*mail.Address{{Email: "ERG+task@sarak.sh"}}

	t.Run("draft without From", func(t *testing.T) {
		if _, err := submissionIdentity(idents, "", nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("the one whose address is From", func(t *testing.T) {
		got, err := submissionIdentity(idents, "", fromAlias)
		if err != nil {
			t.Fatal(err)
		}
		if got != alias {
			t.Fatalf("got %v, want %v", got, alias)
		}
	})

	t.Run("first when none is From", func(t *testing.T) {
		got, err := submissionIdentity(idents, "", []*mail.Address{{Email: "other@sarak.sh"}})
		if err != nil {
			t.Fatal(err)
		}
		if got != agent {
			t.Fatalf("got %v, want %v", got, agent)
		}
	})

	t.Run("named wins", func(t *testing.T) {
		got, err := submissionIdentity(idents, "i1", fromAlias)
		if err != nil {
			t.Fatal(err)
		}
		if got != agent {
			t.Fatalf("got %v, want %v", got, agent)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		if _, err := submissionIdentity(idents, "nope", fromAlias); err == nil {
			t.Fatal("expected error")
		}
	})
}
