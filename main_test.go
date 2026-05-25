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
