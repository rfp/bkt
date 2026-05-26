package main

import (
	"errors"
	"fmt"
)

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		help()
		return nil
	}

	switch args[0] {
	case "auth":
		return a.Auth(args[1:])
	case "repo":
		repo(a.Config, args[1:])
		return nil
	case "pr":
		return a.PR(args[1:])
	case "pipeline":
		return a.Pipeline(args[1:])
	case "version", "--version", "-v":
		printVersion(a.Stdout)
		return nil
	case "help", "--help", "-h":
		help()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func (a *App) Auth(args []string) error {
	if len(args) == 0 {
		return errors.New("missing auth subcommand")
	}

	switch args[0] {
	case "login":
		return a.AuthLogin()
	case "logout":
		if err := a.AuthLogout(); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "Logged out")
		return nil
	case "status":
		if err := ensureAuthE(a.Config); err != nil {
			return err
		}
		client, err := a.Clients.New(a.Config)
		if err != nil {
			return err
		}
		u, err := client.CurrentUser()
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "Logged in as %s\nEmail: %s\nUsername: %s\nAPI: %s\n", u.DisplayName, a.Config.Email, a.Config.Username, a.Config.APIBaseURL)
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand: %s", args[0])
	}
}

func (a *App) PR(args []string) error {
	if len(args) == 0 {
		return errors.New("missing pr subcommand")
	}

	switch args[0] {
	case "approve":
		repo, err := a.detectRepo("origin")
		if err != nil {
			return err
		}
		result, err := a.PRApprove(args[1:], repo)
		if err != nil {
			return err
		}
		if result.Message != "" {
			fmt.Fprintln(a.Stdout, result.Message)
		}
		return nil
	case "merge":
		repo, err := a.detectRepo("origin")
		if err != nil {
			return err
		}
		result, err := a.PRMerge(args[1:], repo)
		if err != nil {
			return err
		}
		if result.Message != "" {
			fmt.Fprintln(a.Stdout, result.Message)
		}
		return nil
	default:
		pr(a.Config, args)
		return nil
	}
}

func (a *App) Pipeline(args []string) error {
	if len(args) == 0 {
		return errors.New("missing pipeline subcommand")
	}

	switch args[0] {
	case "run":
		repo, err := a.detectRepo("origin")
		if err != nil {
			return err
		}
		result, err := a.PipelineRun(args[1:], repo)
		if err != nil {
			return err
		}
		if result.Message != "" {
			fmt.Fprintln(a.Stdout, result.Message)
		}
		return nil
	default:
		pipeline(a.Config, args)
		return nil
	}
}

func (a *App) detectRepo(remote string) (RepoRef, error) {
	u, err := a.Git.RemoteURL(remote)
	if err != nil {
		return RepoRef{}, err
	}
	return parseBitbucketRemoteURL(u)
}
