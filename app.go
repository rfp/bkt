package main

import (
	"fmt"
	"io"
	"os"
)

type InputReader interface {
	Line(prompt string) string
	Secret(prompt string) (string, error)
	Confirm(prompt string) bool
}

type CredentialStore interface {
	Save(account, token string) error
	Load(account string) (string, error)
	Delete(account string) error
}

type GitRunner interface {
	RemoteURL(remote string) (string, error)
	CurrentBranch() (string, error)
	Run(name string, args ...string) error
}

type APIClient interface {
	userClient
	writeCommandClient
}

type ClientFactory interface {
	New(cfg Config) (APIClient, error)
}

type App struct {
	Config Config

	Input       InputReader
	Credentials CredentialStore
	Git         GitRunner
	Clients     ClientFactory

	Stdout io.Writer
	Stderr io.Writer
}

func NewDefaultApp(cfg Config) *App {
	return &App{
		Config:      cfg,
		Input:       TerminalInput{},
		Credentials: KeyringStore{},
		Git:         ShellGitRunner{},
		Clients:     BitbucketClientFactory{},
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
}

type TerminalInput struct{}

func (TerminalInput) Line(prompt string) string {
	return readLine(prompt)
}

func (TerminalInput) Secret(prompt string) (string, error) {
	return readSecret(prompt)
}

func (TerminalInput) Confirm(prompt string) bool {
	return confirmAction(prompt)
}

type KeyringStore struct{}

func (KeyringStore) Save(account, token string) error {
	return saveToken(account, token)
}

func (KeyringStore) Load(account string) (string, error) {
	return loadToken(account)
}

func (KeyringStore) Delete(account string) error {
	return deleteToken(account)
}

type ShellGitRunner struct{}

func (ShellGitRunner) RemoteURL(remote string) (string, error) {
	return remoteURL(remote)
}

func (ShellGitRunner) CurrentBranch() (string, error) {
	return currentBranch()
}

func (ShellGitRunner) Run(name string, args ...string) error {
	return run(name, args...)
}

type BitbucketClientFactory struct{}

func (BitbucketClientFactory) New(cfg Config) (APIClient, error) {
	return newClient(cfg)
}

func (a *App) AuthLogin() error {
	var err error
	a.Config.APIBaseURL, err = validateAPIBaseURL(a.Config.APIBaseURL)
	if err != nil {
		return err
	}

	a.Config.Email = a.Input.Line("Atlassian account email: ")
	a.Config.Username = a.Input.Line("Bitbucket username, optional: ")
	token, err := a.Input.Secret("API token: ")
	if err != nil {
		return err
	}
	a.Config.Token = token
	a.Config.Workspace = a.Input.Line("Default workspace, optional: ")

	if a.Config.Email == "" || a.Config.Token == "" {
		return fmt.Errorf("email and token are required")
	}

	client, err := a.Clients.New(a.Config)
	if err != nil {
		return err
	}
	u, err := client.CurrentUser()
	if err != nil {
		return err
	}
	if a.Config.Username == "" {
		a.Config.Username = u.Nickname
	}
	if err := a.Credentials.Save(a.Config.Email, a.Config.Token); err != nil {
		return fmt.Errorf("could not store API token in keychain: %w", err)
	}
	if err := saveConfig(a.Config); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "Logged in as %s (%s)\n", u.DisplayName, a.Config.Email)
	return nil
}

func (a *App) AuthLogout() error {
	if a.Config.Email != "" {
		if err := a.Credentials.Delete(a.Config.Email); err != nil {
			return fmt.Errorf("could not remove API token from keychain: %w", err)
		}
	}
	return deleteConfig()
}

func (a *App) PRApprove(args []string, repo RepoRef) (writeCommandResult, error) {
	client, err := a.Clients.New(a.Config)
	if err != nil {
		return writeCommandResult{}, err
	}
	return runPRApproveCommand(a.Config, args, writeCommandDeps{
		Repo:    repo,
		Client:  client,
		Confirm: a.Input.Confirm,
	})
}

func (a *App) PRMerge(args []string, repo RepoRef) (writeCommandResult, error) {
	client, err := a.Clients.New(a.Config)
	if err != nil {
		return writeCommandResult{}, err
	}
	return runPRMergeCommand(a.Config, args, writeCommandDeps{
		Repo:    repo,
		Client:  client,
		Confirm: a.Input.Confirm,
	})
}

func (a *App) PipelineRun(args []string, repo RepoRef) (writeCommandResult, error) {
	client, err := a.Clients.New(a.Config)
	if err != nil {
		return writeCommandResult{}, err
	}
	return runPipelineRunCommand(a.Config, args, writeCommandDeps{
		Repo:          repo,
		Client:        client,
		Confirm:       a.Input.Confirm,
		CurrentBranch: a.Git.CurrentBranch,
	})
}
