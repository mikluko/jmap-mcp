package main

import (
	"runtime/debug"
	"testing"
)

func TestReportedVersion(t *testing.T) {
	module := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "sarak.sh/mikluko/jmap-mcp", Version: v}}
	}
	tests := []struct {
		name   string
		linked string
		info   *debug.BuildInfo
		want   string
	}{
		{"a release build keeps its linked version", "0.15.1", module("v0.16.0"), "0.15.1"},
		{"a module build reports the module version as a release does", "0.0.0", module("v0.15.1"), "0.15.1"},
		{"a pseudo-version loses only its v", "0.0.0", module("v0.15.2-0.20261113120000-9778d70d1a2b+dirty"), "0.15.2-0.20261113120000-9778d70d1a2b+dirty"},
		{"a build without a module version says so", "0.0.0", module("(devel)"), "(devel)"},
		{"an empty module version leaves the placeholder", "0.0.0", module(""), "0.0.0"},
		{"no build info leaves the placeholder", "0.0.0", nil, "0.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportedVersion(tt.linked, tt.info); got != tt.want {
				t.Errorf("reportedVersion(%q, %v) = %q, want %q", tt.linked, tt.info, got, tt.want)
			}
		})
	}
}
