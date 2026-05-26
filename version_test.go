package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintVersionUsesDefaults(t *testing.T) {
	var out bytes.Buffer

	printVersion(&out)

	got := out.String()
	if !strings.Contains(got, "bkt version dev") {
		t.Fatalf("expected default version output, got %q", got)
	}
	if !strings.Contains(got, "commit unknown") {
		t.Fatalf("expected default commit output, got %q", got)
	}
	if !strings.Contains(got, "built unknown") {
		t.Fatalf("expected default build output, got %q", got)
	}
}

func TestAppRunRoutesVersionCommand(t *testing.T) {
	app, _, _, stdout := newTestApp(&fakeAPIClient{})

	if err := app.Run([]string{"version"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "bkt version dev") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestAppRunRoutesVersionFlag(t *testing.T) {
	app, _, _, stdout := newTestApp(&fakeAPIClient{})

	if err := app.Run([]string{"--version"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "bkt version dev") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}
