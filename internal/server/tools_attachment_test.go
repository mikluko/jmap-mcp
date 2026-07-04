package server

import (
	"strings"
	"testing"

	"github.com/mikluko/jmap/mail/email"
)

func TestSelectAttachment(t *testing.T) {
	one := &email.BodyPart{BlobID: "b1", Name: "a.pdf", Type: "application/pdf", Size: 10}
	two := &email.BodyPart{BlobID: "b2", Name: "b.png", Type: "image/png", Size: 20}

	t.Run("no attachments", func(t *testing.T) {
		if _, err := selectAttachment(nil, ""); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("sole attachment without blob_id", func(t *testing.T) {
		part, err := selectAttachment([]*email.BodyPart{one}, "")
		if err != nil {
			t.Fatal(err)
		}
		if part != one {
			t.Fatalf("got %v, want %v", part, one)
		}
	})

	t.Run("multiple attachments require blob_id", func(t *testing.T) {
		_, err := selectAttachment([]*email.BodyPart{one, two}, "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "b.png") {
			t.Fatalf("error should list attachments, got: %v", err)
		}
	})

	t.Run("match by blob_id", func(t *testing.T) {
		part, err := selectAttachment([]*email.BodyPart{one, two}, "b2")
		if err != nil {
			t.Fatal(err)
		}
		if part != two {
			t.Fatalf("got %v, want %v", part, two)
		}
	})

	t.Run("unknown blob_id", func(t *testing.T) {
		_, err := selectAttachment([]*email.BodyPart{one, two}, "nope")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "a.pdf") {
			t.Fatalf("error should list attachments, got: %v", err)
		}
	})
}

func TestFormatAttachmentList(t *testing.T) {
	parts := []*email.BodyPart{
		{BlobID: "b1", Name: "report.pdf", Type: "application/pdf", Size: 1234},
		{BlobID: "b2", Type: "image/png", Size: 20},
	}
	got := formatAttachmentList(parts, "  ")
	want := "  report.pdf (application/pdf, 1234 bytes) [blob: b1]\n  (unnamed) (image/png, 20 bytes) [blob: b2]"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
