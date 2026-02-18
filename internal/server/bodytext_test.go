package server

import (
	"strings"
	"testing"
)

func TestStripBlockquotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantIn   string // substring that must be present
		wantOut  string // substring that must be absent
	}{
		{
			name:    "removes blockquote content",
			input:   `<html><body><p>Hello</p><blockquote><p>Quoted reply</p></blockquote><p>Goodbye</p></body></html>`,
			wantIn:  "Hello",
			wantOut: "Quoted reply",
		},
		{
			name:    "removes nested blockquotes",
			input:   `<html><body><p>Original</p><blockquote><p>Level 1</p><blockquote><p>Level 2</p></blockquote></blockquote></body></html>`,
			wantIn:  "Original",
			wantOut: "Level 2",
		},
		{
			name:    "preserves html without blockquotes",
			input:   `<html><body><p>Just a paragraph</p><div>And a div</div></body></html>`,
			wantIn:  "Just a paragraph",
			wantOut: "",
		},
		{
			name:    "handles empty input",
			input:   "",
			wantIn:  "",
			wantOut: "",
		},
		{
			name:    "handles plain text (no HTML tags)",
			input:   "Just plain text, no tags here.",
			wantIn:  "Just plain text",
			wantOut: "",
		},
		{
			name:    "preserves content around blockquote",
			input:   `<div>Before</div><blockquote>Quoted</blockquote><div>After</div>`,
			wantIn:  "After",
			wantOut: "Quoted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripBlockquotes(tt.input)
			if tt.wantIn != "" && !strings.Contains(got, tt.wantIn) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.wantIn, got)
			}
			if tt.wantOut != "" && strings.Contains(got, tt.wantOut) {
				t.Errorf("expected output NOT to contain %q, got:\n%s", tt.wantOut, got)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		check func(t *testing.T, got string)
	}{
		{
			name:  "short text unchanged",
			input: "Hello world",
			limit: 100,
			check: func(t *testing.T, got string) {
				if got != "Hello world" {
					t.Errorf("expected no truncation, got: %q", got)
				}
			},
		},
		{
			name:  "exact limit unchanged",
			input: "12345",
			limit: 5,
			check: func(t *testing.T, got string) {
				if got != "12345" {
					t.Errorf("expected no truncation, got: %q", got)
				}
			},
		},
		{
			name:  "truncates at newline boundary",
			input: "Line one\nLine two\nLine three\nLine four\nLine five\nLine six",
			limit: 50,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "[... body truncated ...]") {
					t.Error("expected truncation marker")
				}
				if strings.Contains(got, "Line four") {
					t.Error("expected Line four to be truncated")
				}
				if !strings.Contains(got, "Line one") {
					t.Error("expected Line one to be preserved")
				}
			},
		},
		{
			name:  "very small limit degrades gracefully",
			input: "This is a longer text that cannot fit",
			limit: 5,
			check: func(t *testing.T, got string) {
				// With limit smaller than marker, just return marker
				if got != truncationMarker {
					t.Errorf("expected just marker for tiny limit, got: %q", got)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateBody(tt.input, tt.limit)
			tt.check(t, got)
		})
	}
}

func TestPrepareBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxChars int
		check    func(t *testing.T, got string)
	}{
		{
			name: "strips text-level quotes",
			input: `Hey, this is the original message.

On Mon, Jan 1, 2024 at 10:00 AM Someone <someone@example.com> wrote:
> This is the quoted reply.
> It should be removed.`,
			maxChars: 0,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "original message") {
					t.Error("expected original message preserved")
				}
				if strings.Contains(got, "quoted reply") {
					t.Error("expected quoted reply to be stripped")
				}
			},
		},
		{
			name: "preserves forwarded messages",
			input: `Check out this forwarded message:

---------- Forwarded message ---------
From: Someone <someone@example.com>
Subject: Important thing

The forwarded content here.`,
			maxChars: 0,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "forwarded") {
					t.Error("expected forwarded content to be preserved")
				}
			},
		},
		{
			name:     "truncates after stripping",
			input:    strings.Repeat("word ", 2000),
			maxChars: 100,
			check: func(t *testing.T, got string) {
				if len(got) > 100 {
					t.Errorf("expected body <= 100 chars, got %d", len(got))
				}
				if !strings.Contains(got, "[... body truncated ...]") {
					t.Error("expected truncation marker")
				}
			},
		},
		{
			name:     "uses default limit when 0 passed",
			input:    strings.Repeat("x", DefaultMaxBodyChars+100),
			maxChars: 0,
			check: func(t *testing.T, got string) {
				if len(got) > DefaultMaxBodyChars {
					t.Errorf("expected body <= %d chars, got %d", DefaultMaxBodyChars, len(got))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrepareBody(tt.input, tt.maxChars)
			tt.check(t, got)
		})
	}
}
