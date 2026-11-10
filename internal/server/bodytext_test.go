package server

import (
	"strings"
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		// wantOrder are substrings that must appear, in this order.
		wantOrder []string
		// wantOut are substrings that must be absent.
		wantOut []string
	}{
		{
			name:      "top-posted reply loses quoted history",
			in:        `<html><body><p>Thanks, done.</p><div>On Mon, Jan 1, 2024 at 10:00 AM Someone &lt;s@example.com&gt; wrote:</div><blockquote><p>Can you do it?</p></blockquote></body></html>`,
			wantOrder: []string{"Thanks, done."},
			wantOut:   []string{"Can you do it?", "wrote:"},
		},
		{
			name:      "interleaved reply keeps each question before its answer",
			in:        `<p>Hi,</p><blockquote><p>Q1 first?</p></blockquote><p>A1 yes.</p><blockquote><p>Q2 second?</p></blockquote><p>A2 no.</p>`,
			wantOrder: []string{"Hi,", "> Q1 first?", "A1 yes.", "> Q2 second?", "A2 no."},
		},
		{
			name:      "trailing quote after interleaved ones is dropped",
			in:        `<blockquote>Q1</blockquote><p>A1</p><blockquote>Q2 unanswered</blockquote>`,
			wantOrder: []string{"> Q1", "A1"},
			wantOut:   []string{"Q2"},
		},
		{
			name:      "nested quotes get one marker per level",
			in:        `<blockquote><p>outer</p><blockquote><p>inner</p></blockquote></blockquote><p>answer</p>`,
			wantOrder: []string{"> outer", "> > inner", "answer"},
		},
		{
			name:      "multi-line quote prefixes every line",
			in:        `<blockquote>line one<br>line two</blockquote><p>reply</p>`,
			wantOrder: []string{"> line one", "> line two", "reply"},
		},
		{
			name:    "reply whose only content is a trailing quote",
			in:      `<html><body><blockquote><p>Original text</p></blockquote></body></html>`,
			wantOut: []string{"Original text", ">"},
		},
		{
			name:      "html without blockquotes",
			in:        `<html><body><p>Just a paragraph</p><div>And a div</div></body></html>`,
			wantOrder: []string{"Just a paragraph", "And a div"},
		},
		{
			name:      "plain text without tags",
			in:        "Just plain text, no tags here.",
			wantOrder: []string{"Just plain text"},
		},
		{name: "empty input", in: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTMLToText(tt.in)
			rest := got
			for _, w := range tt.wantOrder {
				i := strings.Index(rest, w)
				if i < 0 {
					t.Fatalf("expected %q (in order) in output, got:\n%s", w, got)
				}
				rest = rest[i+len(w):]
			}
			for _, w := range tt.wantOut {
				if strings.Contains(got, w) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", w, got)
				}
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
