package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestAppRunRoutesAuthStatus(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := &App{
		Config:      authenticatedConfig(),
		Input:       &fakeInputReader{},
		Credentials: newFakeCredentialStore(),
		Git:         fakeGitRunner{},
		Clients:     fakeClientFactory{client: &fakeAPIClient{user: User{DisplayName: "Rui", Nickname: "rfp"}}},
		Stdout:      stdout,
		Stderr:      io.Discard,
	}

	if err := app.Run([]string{"auth", "status"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Logged in as Rui") {
		t.Fatalf("expected auth status output, got %q", stdout.String())
	}
}

func TestAppRunRoutesPRApprove(t *testing.T) {
	client := &fakeAPIClient{}
	app, input, _, stdout := newTestApp(client)

	if err := app.Run([]string{"pr", "approve", "123"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if client.approveCalls != 1 || input.confirmCalls != 1 {
		t.Fatalf("expected approve and confirmation, approve=%d confirm=%d", client.approveCalls, input.confirmCalls)
	}
	if !strings.Contains(stdout.String(), "Approved PR #123") {
		t.Fatalf("expected approve output, got %q", stdout.String())
	}
}

func TestAppRunRoutesPRMerge(t *testing.T) {
	client := &fakeAPIClient{}
	app, input, _, stdout := newTestApp(client)

	if err := app.Run([]string{"pr", "merge", "123", "--message", "Ship it"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if client.mergeCalls != 1 || client.mergeMessage != "Ship it" || input.confirmCalls != 1 {
		t.Fatalf("unexpected merge calls=%d message=%q confirm=%d", client.mergeCalls, client.mergeMessage, input.confirmCalls)
	}
	if !strings.Contains(stdout.String(), "Merged PR #123") {
		t.Fatalf("expected merge output, got %q", stdout.String())
	}
}

func TestAppRunRoutesPipelineRun(t *testing.T) {
	client := &fakeAPIClient{}
	app, input, _, stdout := newTestApp(client)

	if err := app.Run([]string{"pipeline", "run", "--branch", "main"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if client.pipelineCalls != 1 || client.pipelineBranch != "main" || input.confirmCalls != 1 {
		t.Fatalf("unexpected pipeline calls=%d branch=%q confirm=%d", client.pipelineCalls, client.pipelineBranch, input.confirmCalls)
	}
	if !strings.Contains(stdout.String(), "Started pipeline #42 for branch main") {
		t.Fatalf("expected pipeline output, got %q", stdout.String())
	}
}

func TestAppRunUnknownCommandReturnsError(t *testing.T) {
	app, _, _, _ := newTestApp(&fakeAPIClient{})

	if err := app.Run([]string{"wat"}); err == nil {
		t.Fatal("expected unknown command error")
	}
}

func TestAppDetectRepoUsesInjectedGitRunner(t *testing.T) {
	app, _, _, _ := newTestApp(&fakeAPIClient{})

	repo, err := app.detectRepo("origin")
	if err != nil {
		t.Fatalf("detectRepo returned error: %v", err)
	}
	if repo.Workspace != "workspace" || repo.Slug != "repo" {
		t.Fatalf("unexpected repo: %+v", repo)
	}
}
