package main

import (
	"errors"
	"flag"
	"fmt"
)

type writeCommandClient interface {
	PR(workspace, repo string, id int) (PullRequest, error)
	ApprovePR(workspace, repo string, id int) error
	MergePR(workspace, repo string, id int, message string) error
	RunPipeline(workspace, repo, branch string) (Pipeline, error)
}

type writeCommandDeps struct {
	Repo          RepoRef
	Client        writeCommandClient
	Confirm       func(prompt string) bool
	CurrentBranch func() (string, error)
}

type writeCommandResult struct {
	Performed bool
	Message   string
}

func runPRApproveCommand(cfg Config, args []string, deps writeCommandDeps) (writeCommandResult, error) {
	if err := ensureAuthE(cfg); err != nil {
		return writeCommandResult{}, err
	}
	if deps.Client == nil {
		return writeCommandResult{}, errors.New("missing command client")
	}

	fs := flag.NewFlagSet("pr approve", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return writeCommandResult{}, err
	}
	id, err := requireIDE(fs.Args())
	if err != nil {
		return writeCommandResult{}, err
	}

	if !*yes {
		if deps.Confirm == nil {
			return writeCommandResult{}, errors.New("missing confirmation function")
		}
		if !deps.Confirm(fmt.Sprintf("Approve PR #%d in %s/%s?", id, deps.Repo.Workspace, deps.Repo.Slug)) {
			return writeCommandResult{Performed: false, Message: "Aborted"}, nil
		}
	}

	if err := deps.Client.ApprovePR(deps.Repo.Workspace, deps.Repo.Slug, id); err != nil {
		return writeCommandResult{}, err
	}
	return writeCommandResult{Performed: true, Message: fmt.Sprintf("Approved PR #%d", id)}, nil
}

func runPRMergeCommand(cfg Config, args []string, deps writeCommandDeps) (writeCommandResult, error) {
	if err := ensureAuthE(cfg); err != nil {
		return writeCommandResult{}, err
	}
	if deps.Client == nil {
		return writeCommandResult{}, errors.New("missing command client")
	}

	fs := flag.NewFlagSet("pr merge", flag.ContinueOnError)
	msg := fs.String("message", "", "merge message")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return writeCommandResult{}, err
	}
	id, err := requireIDE(fs.Args())
	if err != nil {
		return writeCommandResult{}, err
	}

	pr, err := deps.Client.PR(deps.Repo.Workspace, deps.Repo.Slug, id)
	if err != nil {
		return writeCommandResult{}, err
	}

	if !*yes {
		if deps.Confirm == nil {
			return writeCommandResult{}, errors.New("missing confirmation function")
		}
		prompt := fmt.Sprintf("Merge PR #%d (%s) into %s in %s/%s?", id, pr.Title, pr.Destination.Branch.Name, deps.Repo.Workspace, deps.Repo.Slug)
		if !deps.Confirm(prompt) {
			return writeCommandResult{Performed: false, Message: "Aborted"}, nil
		}
	}

	if err := deps.Client.MergePR(deps.Repo.Workspace, deps.Repo.Slug, id, *msg); err != nil {
		return writeCommandResult{}, err
	}
	return writeCommandResult{Performed: true, Message: fmt.Sprintf("Merged PR #%d", id)}, nil
}

func runPipelineRunCommand(cfg Config, args []string, deps writeCommandDeps) (writeCommandResult, error) {
	if err := ensureAuthE(cfg); err != nil {
		return writeCommandResult{}, err
	}
	if deps.Client == nil {
		return writeCommandResult{}, errors.New("missing command client")
	}

	fs := flag.NewFlagSet("pipeline run", flag.ContinueOnError)
	branch := fs.String("branch", "", "branch to run")
	yes := fs.Bool("yes", false, "skip confirmation")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = jsonOut
	if err := fs.Parse(args); err != nil {
		return writeCommandResult{}, err
	}

	if *branch == "" {
		if deps.CurrentBranch == nil {
			return writeCommandResult{}, errors.New("missing current branch function")
		}
		current, err := deps.CurrentBranch()
		if err != nil {
			return writeCommandResult{}, err
		}
		*branch = current
	}

	if !*yes {
		if deps.Confirm == nil {
			return writeCommandResult{}, errors.New("missing confirmation function")
		}
		if !deps.Confirm(fmt.Sprintf("Run pipeline for branch %s in %s/%s?", *branch, deps.Repo.Workspace, deps.Repo.Slug)) {
			return writeCommandResult{Performed: false, Message: "Aborted"}, nil
		}
	}

	pipeline, err := deps.Client.RunPipeline(deps.Repo.Workspace, deps.Repo.Slug, *branch)
	if err != nil {
		return writeCommandResult{}, err
	}
	return writeCommandResult{Performed: true, Message: fmt.Sprintf("Started pipeline #%d for branch %s", pipeline.BuildNumber, *branch)}, nil
}

func requireIDE(args []string) (int, error) {
	if len(args) != 1 {
		return 0, errors.New("missing numeric id")
	}
	id, err := strconvAtoi(args[0])
	if err != nil {
		return 0, err
	}
	return id, nil
}

func ensureAuthE(cfg Config) error {
	if cfg.Email == "" || cfg.Token == "" {
		return errors.New("not authenticated; run: bkt auth login")
	}
	return nil
}

var strconvAtoi = strconv.Atoi
