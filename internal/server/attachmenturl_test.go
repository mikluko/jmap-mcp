package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClaims(exp int64) *attachmentClaims {
	return &attachmentClaims{
		Token:   "secret-token",
		Account: "acc1",
		Blob:    "blob1",
		Name:    "report.pdf",
		Type:    "application/pdf",
		Exp:     exp,
	}
}

func TestAttachmentURLerRoundtrip(t *testing.T) {
	u, err := newAttachmentURLer("test-secret")
	if err != nil {
		t.Fatal(err)
	}

	in := testClaims(time.Now().Add(time.Hour).Unix())
	opaque, err := u.seal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(opaque, "+/=") {
		t.Fatalf("token not URL-safe: %q", opaque)
	}
	if strings.Contains(opaque, "secret-token") {
		t.Fatal("token leaks JMAP bearer token")
	}

	out, err := u.open(opaque)
	if err != nil {
		t.Fatal(err)
	}
	if *out != *in {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestAttachmentURLerSameSecretDifferentInstance(t *testing.T) {
	u1, _ := newAttachmentURLer("shared")
	u2, _ := newAttachmentURLer("shared")

	opaque, err := u1.seal(testClaims(time.Now().Add(time.Hour).Unix()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u2.open(opaque); err != nil {
		t.Fatalf("same secret should open token across instances: %v", err)
	}
}

func TestAttachmentURLerRejects(t *testing.T) {
	u, _ := newAttachmentURLer("test-secret")
	other, _ := newAttachmentURLer("other-secret")

	opaque, err := u.seal(testClaims(time.Now().Add(time.Hour).Unix()))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("tampered", func(t *testing.T) {
		tampered := opaque[:len(opaque)-2] + "aa"
		if tampered == opaque {
			tampered = opaque[:len(opaque)-2] + "bb"
		}
		if _, err := u.open(tampered); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		if _, err := other.open(opaque); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := u.open("not-a-token"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty", func(t *testing.T) {
		if _, err := u.open(""); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAttachmentHandlerAuth(t *testing.T) {
	srv := NewServer("test", "https://jmap.example.invalid/session",
		WithAttachmentURL("test-secret", ""))
	handler := srv.AttachmentHandler()

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	t.Run("invalid token", func(t *testing.T) {
		if rec := get("/attachments/bogus"); rec.Code != 403 {
			t.Fatalf("got %d, want 403", rec.Code)
		}
	})

	t.Run("expired", func(t *testing.T) {
		opaque, err := srv.attachmentURL.seal(testClaims(time.Now().Add(-time.Minute).Unix()))
		if err != nil {
			t.Fatal(err)
		}
		if rec := get("/attachments/" + opaque); rec.Code != 410 {
			t.Fatalf("got %d, want 410", rec.Code)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("POST", "/attachments/x", nil))
		if rec.Code != 405 {
			t.Fatalf("got %d, want 405", rec.Code)
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "attachment"},
		{"report.pdf", "report.pdf"},
		{`ev"il\na/me`, "ev_il_na_me"},
		{"tab\there", "tab_here"},
	}
	for _, tt := range tests {
		if got := sanitizeFilename(tt.in); got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
