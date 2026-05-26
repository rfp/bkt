package main

import "testing"

func TestLocalPRBranchName(t *testing.T) {
	got := localPRBranchName(123)
	want := "pr/123"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
