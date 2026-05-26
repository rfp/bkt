package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	values map[string]string
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{values: map[string]string{}}
}

func key(service, account string) string {
	return service + ":" + account
}

func (f *fakeKeyring) set(service, account, secret string) error {
	f.values[key(service, account)] = secret
	return nil
}

func (f *fakeKeyring) get(service, account string) (string, error) {
	secret, ok := f.values[key(service, account)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return secret, nil
}

func (f *fakeKeyring) delete(service, account string) error {
	k := key(service, account)
	if _, ok := f.values[k]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.values, k)
	return nil
}

type fakeUserClient struct {
	user User
	err  error
}

func (f fakeUserClient) CurrentUser() (User, error) {
	return f.user, f.err
}

func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BKT_CONFIG_DIR", dir)
	return dir
}

func withFakeKeyring(t *testing.T) *fakeKeyring {
	t.Helper()
	fake := newFakeKeyring()
	oldSet := keyringSet
	oldGet := keyringGet
	oldDelete := keyringDelete

	keyringSet = fake.set
	keyringGet = fake.get
	keyringDelete = fake.delete

	t.Cleanup(func() {
		keyringSet = oldSet
		keyringGet = oldGet
		keyringDelete = oldDelete
	})

	return fake
}

func withStdin(t *testing.T, input string) {
	t.Helper()

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("could not create stdin pipe: %v", err)
	}

	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("could not write fake stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("could not close fake stdin writer: %v", err)
	}

	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})
}

func withInputFuncs(t *testing.T, lines []string, secret string, secretErr error) {
	t.Helper()
	oldInputLine := inputLine
	oldInputSecret := inputSecret
	lineIndex := 0

	inputLine = func(prompt string) string {
		if lineIndex >= len(lines) {
			t.Fatalf("unexpected inputLine call for prompt %q", prompt)
		}
		value := lines[lineIndex]
		lineIndex++
		return value
	}
	inputSecret = func(prompt string) (string, error) {
		return secret, secretErr
	}

	t.Cleanup(func() {
		inputLine = oldInputLine
		inputSecret = oldInputSecret
	})
}

func withFakeUserClient(t *testing.T, client userClient, err error) {
	t.Helper()
	old := newUserClient
	newUserClient = func(cfg Config) (userClient, error) {
		return client, err
	}
	t.Cleanup(func() { newUserClient = old })
}

func TestConfirmActionDefaultsToNo(t *testing.T) {
	withStdin(t, "\n")

	if confirmAction("Do it?") {
		t.Fatal("empty confirmation must default to no")
	}
}

func TestConfirmActionAcceptsOnlyYes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "y", input: "y\n", want: true},
		{name: "yes", input: "yes\n", want: true},
		{name: "uppercase yes", input: "YES\n", want: true},
		{name: "n", input: "n\n", want: false},
		{name: "random text", input: "definitely\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStdin(t, tt.input)
			if got := confirmAction("Do it?"); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestAuthLoginUsesSecretInputAndStoresTokenInKeyring(t *testing.T) {
	dir := withTempConfigDir(t)
	fake := withFakeKeyring(t)
	withInputFuncs(t, []string{"rui@example.com", "", "workspace"}, "secret-token", nil)
	withFakeUserClient(t, fakeUserClient{user: User{DisplayName: "Rui", Nickname: "rfp"}}, nil)

	if err := authLogin(Config{}); err != nil {
		t.Fatalf("authLogin returned error: %v", err)
	}

	storedToken, err := fake.get(keyringService, "rui@example.com")
	if err != nil {
		t.Fatalf("expected token in fake keyring: %v", err)
	}
	if storedToken != "secret-token" {
		t.Fatalf("expected keyring token %q, got %q", "secret-token", storedToken)
	}

	content, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("could not read config: %v", err)
	}
	configText := string(content)
	if strings.Contains(configText, "token=") || strings.Contains(configText, "secret-token") {
		t.Fatalf("config must not contain token data, got:\n%s", configText)
	}
	if !strings.Contains(configText, "username=rfp") {
		t.Fatalf("expected username from Bitbucket user nickname, got:\n%s", configText)
	}
}

func TestAuthLoginFailsWhenSecretInputFails(t *testing.T) {
	withTempConfigDir(t)
	withFakeKeyring(t)
	withInputFuncs(t, []string{"rui@example.com", "rfp"}, "", errors.New("not a terminal"))
	withFakeUserClient(t, fakeUserClient{user: User{DisplayName: "Rui", Nickname: "rfp"}}, nil)

	if err := authLogin(Config{}); err == nil {
		t.Fatal("expected authLogin to fail when secret input fails")
	}
}

func TestAuthLoginFailsWhenKeyringStorageFails(t *testing.T) {
	withTempConfigDir(t)
	withInputFuncs(t, []string{"rui@example.com", "rfp", "workspace"}, "secret-token", nil)
	withFakeUserClient(t, fakeUserClient{user: User{DisplayName: "Rui", Nickname: "rfp"}}, nil)

	oldSet := keyringSet
	keyringSet = func(service, account, secret string) error {
		return errors.New("keychain unavailable")
	}
	t.Cleanup(func() { keyringSet = oldSet })

	if err := authLogin(Config{}); err == nil {
		t.Fatal("expected authLogin to fail when keyring storage fails")
	}
}

func TestAuthLogoutRemovesConfigAndToken(t *testing.T) {
	dir := withTempConfigDir(t)
	fake := withFakeKeyring(t)
	if err := fake.set(keyringService, "rui@example.com", "secret-token"); err != nil {
		t.Fatalf("could not seed fake keyring: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("email=rui@example.com\n"), 0600); err != nil {
		t.Fatalf("could not write config: %v", err)
	}

	if err := authLogout(Config{Email: "rui@example.com"}); err != nil {
		t.Fatalf("authLogout returned error: %v", err)
	}
	if _, err := fake.get(keyringService, "rui@example.com"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("expected token to be removed from keyring, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config")); !os.IsNotExist(err) {
		t.Fatalf("expected config to be removed, got: %v", err)
	}
}

func TestValidateAPIBaseURLAllowsOnlyBitbucketCloud(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty uses default", raw: "", want: defaultAPIBaseURL},
		{name: "canonical", raw: "https://api.bitbucket.org/2.0", want: defaultAPIBaseURL},
		{name: "trailing slash", raw: "https://api.bitbucket.org/2.0/", want: defaultAPIBaseURL},
		{name: "surrounding whitespace", raw: "  https://api.bitbucket.org/2.0  ", want: defaultAPIBaseURL},
		{name: "http rejected", raw: "http://api.bitbucket.org/2.0", wantErr: true},
		{name: "wrong host rejected", raw: "https://example.com/2.0", wantErr: true},
		{name: "wrong path rejected", raw: "https://api.bitbucket.org/1.0", wantErr: true},
		{name: "missing scheme rejected", raw: "api.bitbucket.org/2.0", wantErr: true},
		{name: "query rejected", raw: "https://api.bitbucket.org/2.0?x=1", wantErr: true},
		{name: "fragment rejected", raw: "https://api.bitbucket.org/2.0#token", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAPIBaseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNewClientRejectsInvalidAPIBaseURL(t *testing.T) {
	_, err := newClient(Config{APIBaseURL: "https://example.com/2.0"})
	if err == nil {
		t.Fatal("expected newClient to reject non-Bitbucket API host")
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{name: "normal branch", branch: "feature/login"},
		{name: "branch with slash and dash", branch: "bugfix/fix-login-123"},
		{name: "empty rejected", branch: "", wantErr: true},
		{name: "leading dash rejected", branch: "-danger", wantErr: true},
		{name: "leading whitespace rejected", branch: " feature/login", wantErr: true},
		{name: "trailing whitespace rejected", branch: "feature/login ", wantErr: true},
		{name: "newline rejected", branch: "feature/login\nnext", wantErr: true},
		{name: "carriage return rejected", branch: "feature/login\rnext", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for branch %q", tt.branch)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for branch %q: %v", tt.branch, err)
			}
		})
	}
}

func TestFormatAPIErrorTruncatesAndRedactsToken(t *testing.T) {
	body := strings.Repeat("A", maxAPIErrorBody+50) + " secret-token"
	err := formatAPIError(403, []byte(body), "secret-token")
	if err == nil {
		t.Fatal("expected formatted API error")
	}
	msg := err.Error()
	if strings.Contains(msg, "secret-token") {
		t.Fatalf("error message leaked token: %s", msg)
	}
	if len(msg) > len("bitbucket API error 403: ")+maxAPIErrorBody+3 {
		t.Fatalf("error message was not truncated: length %d", len(msg))
	}
}

func TestFormatAPIErrorHandlesEmptyBody(t *testing.T) {
	err := formatAPIError(500, []byte("   "), "")
	if err == nil || err.Error() != "bitbucket API error 500" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestURLValidatesAbsoluteURLs(t *testing.T) {
	client, err := newClient(Config{APIBaseURL: defaultAPIBaseURL})
	if err != nil {
		t.Fatalf("newClient returned error: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "relative path allowed",
			path: "/repositories/workspace/repo/pullrequests?pagelen=50",
			want: defaultAPIBaseURL + "/repositories/workspace/repo/pullrequests?pagelen=50",
		},
		{
			name: "absolute Bitbucket Cloud pagination URL allowed",
			path: "https://api.bitbucket.org/2.0/repositories/workspace/repo/pullrequests?page=2",
			want: "https://api.bitbucket.org/2.0/repositories/workspace/repo/pullrequests?page=2",
		},
		{
			name:    "different host rejected",
			path:    "https://example.com/2.0/repositories/workspace/repo/pullrequests?page=2",
			wantErr: true,
		},
		{
			name:    "http scheme rejected",
			path:    "http://api.bitbucket.org/2.0/repositories/workspace/repo/pullrequests?page=2",
			wantErr: true,
		},
		{
			name:    "wrong API path rejected",
			path:    "https://api.bitbucket.org/1.0/repositories/workspace/repo/pullrequests?page=2",
			wantErr: true,
		},
		{
			name:    "fragment rejected",
			path:    "https://api.bitbucket.org/2.0/repositories/workspace/repo/pullrequests?page=2#token",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.requestURL(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidAPIBaseURL(t *testing.T) {
	dir := withTempConfigDir(t)
	config := "email=rui@example.com\nusername=rfp\nworkspace=workspace\napi_base_url=https://example.com/2.0\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0600); err != nil {
		t.Fatalf("could not write config: %v", err)
	}

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected loadConfig to reject invalid api_base_url")
	}
}

func TestSaveConfigDoesNotWriteToken(t *testing.T) {
	dir := withTempConfigDir(t)

	cfg := Config{
		Email:      "rui@example.com",
		Username:   "rfp",
		Token:      "super-secret-token",
		Workspace:  "workspace",
		APIBaseURL: "https://api.bitbucket.org/2.0",
	}

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("could not read config file: %v", err)
	}

	configText := string(content)
	if strings.Contains(configText, "token=") {
		t.Fatalf("config file must not contain token=, got:\n%s", configText)
	}
	if strings.Contains(configText, cfg.Token) {
		t.Fatal("config file contains the API token value")
	}
}

func TestSaveConfigNormalizesAPIBaseURL(t *testing.T) {
	dir := withTempConfigDir(t)

	cfg := Config{
		Email:      "rui@example.com",
		Username:   "rfp",
		Workspace:  "workspace",
		APIBaseURL: "https://api.bitbucket.org/2.0/",
	}

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("could not read config file: %v", err)
	}

	if !strings.Contains(string(content), "api_base_url="+defaultAPIBaseURL) {
		t.Fatalf("expected normalized API base URL, got:\n%s", string(content))
	}
}

func TestLoadConfigLoadsTokenFromKeyring(t *testing.T) {
	dir := withTempConfigDir(t)
	fake := withFakeKeyring(t)

	account := "rui@example.com"
	storedToken := "token-from-keyring"
	if err := fake.set(keyringService, account, storedToken); err != nil {
		t.Fatalf("could not seed fake keyring: %v", err)
	}

	config := "email=rui@example.com\nusername=rfp\nworkspace=workspace\napi_base_url=https://api.bitbucket.org/2.0\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0600); err != nil {
		t.Fatalf("could not write config: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.Token != storedToken {
		t.Fatalf("expected token %q, got %q", storedToken, cfg.Token)
	}
}

func TestLoadConfigMigratesLegacyTokenToKeyring(t *testing.T) {
	dir := withTempConfigDir(t)
	fake := withFakeKeyring(t)

	legacyToken := "legacy-plain-text-token"
	config := "email=rui@example.com\nusername=rfp\ntoken=" + legacyToken + "\nworkspace=workspace\napi_base_url=https://api.bitbucket.org/2.0\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(config), 0600); err != nil {
		t.Fatalf("could not write legacy config: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.Token != legacyToken {
		t.Fatalf("expected migrated token %q, got %q", legacyToken, cfg.Token)
	}

	migratedToken, err := fake.get(keyringService, "rui@example.com")
	if err != nil {
		t.Fatalf("expected token to be migrated to keyring: %v", err)
	}
	if migratedToken != legacyToken {
		t.Fatalf("expected keyring token %q, got %q", legacyToken, migratedToken)
	}

	content, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("could not read rewritten config: %v", err)
	}
	configText := string(content)
	if strings.Contains(configText, "token=") {
		t.Fatalf("migrated config must not contain token=, got:\n%s", configText)
	}
	if strings.Contains(configText, legacyToken) {
		t.Fatal("migrated config still contains the legacy token value")
	}
}

func TestDeleteTokenIgnoresMissingToken(t *testing.T) {
	withFakeKeyring(t)

	if err := deleteToken("missing@example.com"); err != nil {
		t.Fatalf("deleteToken should ignore missing token, got: %v", err)
	}
}

func TestDeleteTokenReturnsUnexpectedError(t *testing.T) {
	oldDelete := keyringDelete
	keyringDelete = func(service, account string) error {
		return errors.New("keychain unavailable")
	}
	t.Cleanup(func() { keyringDelete = oldDelete })

	if err := deleteToken("rui@example.com"); err == nil {
		t.Fatal("expected deleteToken to return unexpected keyring errors")
	}
}
