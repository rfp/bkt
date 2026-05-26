package main

import "testing"

func TestShouldUseAppEntrypointSkipsGoTestBinary(t *testing.T) {
	if shouldUseAppEntrypoint([]string{"bkt.test"}) {
		t.Fatal("expected App entrypoint to be disabled for go test binary")
	}
}

func TestShouldUseAppEntrypointUsesAppForRuntimeBinary(t *testing.T) {
	t.Setenv("BKT_USE_LEGACY_MAIN", "")

	if !shouldUseAppEntrypoint([]string{"bkt", "auth", "status"}) {
		t.Fatal("expected App entrypoint to be enabled for runtime binary")
	}
}

func TestShouldUseAppEntrypointCanUseLegacyMain(t *testing.T) {
	t.Setenv("BKT_USE_LEGACY_MAIN", "1")

	if shouldUseAppEntrypoint([]string{"bkt", "auth", "status"}) {
		t.Fatal("expected App entrypoint to be disabled when BKT_USE_LEGACY_MAIN=1")
	}
}
