package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type fakeInputReader struct {
	lines        []string
	secret       string
	secretErr    error
	confirm      bool
	confirmCalls int
}

func (f *fakeInputReader) Line(prompt string) string {
	if len(f.lines) == 0 {
		return ""
	}
	value := f.lines[0]
	f.lines = f.lines[1:]
	return value
}

func (f *fakeInputReader) Secret(prompt string) (string, error) {
	return f.secret, f.secretErr
}

func (f *fakeInputReader) Confirm(prompt string) bool {
	f.confirmCalls++
	return f.confirm
}

type fakeCredentialStore struct {
	values      map[string]string
	saveErr     error
	deleteErr   error
	deleteCalls int
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{values: map[string]string{}}
}

func (f *fakeCredentialStore) Save(account, token string) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.values[account] = token
	return nil
}

func (f *fakeCredentialStore) Load(account string) (string, error) {
	value, ok := f.values[account]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (f *fakeCredentialStore) Delete(account string) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.values, account)
	return nil
}

type fakeGitRunner struct {
	currentBranch string
	currentErr    error
}

func (f fakeGitRunner) RemoteURL(remote string) (string, error) {
	return "git@bitbucket.org:workspace/repo.git", nil
}

func (f fakeGitRunner) CurrentBranch() (string, error) {
	if f.currentErr != nil {
		return "", f.currentErr
	}
	if f.currentBranch == "" {
		return "main", nil
	}
	return f.currentBranch, nil
}

func (f fakeGitRunner) Run(name string, args ...string) error {
	return nil
}

type fakeClientFactory struct {
	client APIClient
	err    error
}

func (f fakeClientFactory) New(cfg Config) (APIClient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

func newTestApp(client *fakeAPIClient) (*App, *fakeInputReader, *fakeCredentialStore, *bytes.Buffer) {
	input := &fakeInputReader{confirm: true}
	creds := newFakeCredentialStore()
	stdout := &bytes.Buffer{}
	app := &App{
		Config:      authenticatedConfig(),
		Input:       input,
		Credentials: creds,
		Git:         fakeGitRunner{currentBranch: "feature/current"},
		Clients:     fakeClientFactory{client: client},
		Stdout:      stdout,
		Stderr:      io.Discard,
	}
	return app, input, creds, stdout
}

func TestAppAuthLoginUsesInjectedDependencies(t *testing.T) {
	input := &fakeInputReader{
		lines:  []string{"rui@example.com", "", "workspace"},
		secret: "secret-token",
	}
	creds := newFakeCredentialStore()
	stdout := &bytes.Buffer{}
	app := &App{
		Config:      Config{APIBaseURL: defaultAPIBaseURL},
		Input:       input,
		Credentials: creds,
		Git:         fakeGitRunner{},
		Clients: fakeClientFactory{client: &fakeAPIClient{
			user: User{DisplayName: "Rui", Nickname: "rfp"},
		}},
		Stdout: stdout,
		Stderr: io.Discard,
	}

	withTempConfigDir(t)
	if err := app.AuthLogin(); err != nil {
		t.Fatalf("AuthLogin returned error: %v", err)
	}
	if got := creds.values["rui@example.com"]; got != "secret-token" {
		t.Fatalf("expected token in credential store, got %q", got)
	}
	if app.Config.Username != "rfp" {
		t.Fatalf("expected username from API user nickname, got %q", app.Config.Username)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Logged in as Rui")) {
		t.Fatalf("expected login message, got %q", stdout.String())
	}
}

func TestAppAuthLoginFailsWhenCredentialStoreFails(t *testing.T) {
	input := &fakeInputReader{lines: []string{"rui@example.com", "rfp", "workspace"}, secret: "secret-token"}
	creds := newFakeCredentialStore()
	creds.saveErr = errors.New("store unavailable")
	app := &App{
		Config:      Config{APIBaseURL: defaultAPIBaseURL},
		Input:       input,
		Credentials: creds,
		Git:         fakeGitRunner{},
		Clients:     fakeClientFactory{client: &fakeAPIClient{user: User{DisplayName: "Rui", Nickname: "rfp"}}},
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	}

	withTempConfigDir(t)
	if err := app.AuthLogin(); err == nil {
		t.Fatal("expected AuthLogin to fail when credential store fails")
	}
}

func TestAppAuthLogoutUsesCredentialStore(t *testing.T) {
	creds := newFakeCredentialStore()
	creds.values["rui@example.com"] = "secret-token"
	app := &App{
		Config:      Config{Email: "rui@example.com"},
		Input:       &fakeInputReader{},
		Credentials: creds,
		Git:         fakeGitRunner{},
		Clients:     fakeClientFactory{client: &fakeAPIClient{}},
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	}

	withTempConfigDir(t)
	if err := app.AuthLogout(); err != nil {
		t.Fatalf("AuthLogout returned error: %v", err)
	}
	if creds.deleteCalls != 1 {
		t.Fatalf("expected one delete call, got %d", creds.deleteCalls)
	}
}

func TestAppPRApproveUsesInjectedConfirmationAndClient(t *testing.T) {
	client := &fakeAPIClient{}
	app, input, _, _ := newTestApp(client)

	result, err := app.PRApprove([]string{"123"}, RepoRef{Workspace: "workspace", Slug: "repo"})
	if err != nil {
		t.Fatalf("PRApprove returned error: %v", err)
	}
	if !result.Performed || client.approveCalls != 1 || input.confirmCalls != 1 {
		t.Fatalf("unexpected result=%+v calls=%d confirm=%d", result, client.approveCalls, input.confirmCalls)
	}
}

func TestAppPRApproveYesSkipsInjectedConfirmation(t *testing.T) {
	client := &fakeAPIClient{}
	app, input, _, _ := newTestApp(client)

	result, err := app.PRApprove([]string{"123", "--yes"}, RepoRef{Workspace: "workspace", Slug: "repo"})
	if err != nil {
		t.Fatalf("PRApprove returned error: %v", err)
	}
	if !result.Performed || client.approveCalls != 1 || input.confirmCalls != 0 {
		t.Fatalf("unexpected result=%+v calls=%d confirm=%d", result, client.approveCalls, input.confirmCalls)
	}
}

func TestAppPipelineRunUsesInjectedCurrentBranch(t *testing.T) {
	client := &fakeAPIClient{}
	app, _, _, _ := newTestApp(client)

	result, err := app.PipelineRun([]string{"--yes"}, RepoRef{Workspace: "workspace", Slug: "repo"})
	if err != nil {
		t.Fatalf("PipelineRun returned error: %v", err)
	}
	if !result.Performed || client.pipelineBranch != "feature/current" {
		t.Fatalf("unexpected result=%+v branch=%q", result, client.pipelineBranch)
	}
}

type fakeAPIClient struct {
	fakeWriteClient
	user User
}

func (f *fakeAPIClient) CurrentUser() (User, error) {
	if f.user.DisplayName == "" {
		return User{DisplayName: "Rui", Nickname: "rfp"}, nil
	}
	return f.user, nil
}
