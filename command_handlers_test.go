package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeWriteClient struct {
	approveCalls     int
	approveID        int
	mergeCalls       int
	mergeID          int
	mergeMessage     string
	pipelineCalls    int
	pipelineBranch   string
	prCalls          int
	pr               PullRequest
	approveErr       error
	mergeErr         error
	pipelineErr      error
	prErr            error
	pipelineResponse Pipeline
}

func (f *fakeWriteClient) PR(workspace, repo string, id int) (PullRequest, error) {
	f.prCalls++
	if f.prErr != nil {
		return PullRequest{}, f.prErr
	}
	if f.pr.ID == 0 {
		f.pr = PullRequest{ID: id, Title: "Test PR", Destination: PRSide{Branch: BranchRef{Name: "main"}}}
	}
	return f.pr, nil
}

func (f *fakeWriteClient) ApprovePR(workspace, repo string, id int) error {
	f.approveCalls++
	f.approveID = id
	return f.approveErr
}

func (f *fakeWriteClient) MergePR(workspace, repo string, id int, message string) error {
	f.mergeCalls++
	f.mergeID = id
	f.mergeMessage = message
	return f.mergeErr
}

func (f *fakeWriteClient) RunPipeline(workspace, repo, branch string) (Pipeline, error) {
	f.pipelineCalls++
	f.pipelineBranch = branch
	if f.pipelineErr != nil {
		return Pipeline{}, f.pipelineErr
	}
	if f.pipelineResponse.BuildNumber == 0 {
		f.pipelineResponse = Pipeline{BuildNumber: 42}
	}
	return f.pipelineResponse, nil
}

func testWriteDeps(client *fakeWriteClient, confirm func(prompt string) bool) writeCommandDeps {
	return writeCommandDeps{
		Repo:          RepoRef{Workspace: "workspace", Slug: "repo"},
		Client:        client,
		Confirm:       confirm,
		CurrentBranch: func() (string, error) { return "current-branch", nil },
	}
}

func authenticatedConfig() Config {
	return Config{Email: "rui@example.com", Token: "token", APIBaseURL: defaultAPIBaseURL}
}

func TestRunPRApproveCommandAsksForConfirmationByDefault(t *testing.T) {
	client := &fakeWriteClient{}
	confirmCalled := false

	result, err := runPRApproveCommand(authenticatedConfig(), []string{"123"}, testWriteDeps(client, func(prompt string) bool {
		confirmCalled = true
		if !strings.Contains(prompt, "Approve PR #123") {
			t.Fatalf("unexpected prompt: %s", prompt)
		}
		return true
	}))
	if err != nil {
		t.Fatalf("runPRApproveCommand returned error: %v", err)
	}
	if !confirmCalled {
		t.Fatal("expected confirmation to be requested")
	}
	if !result.Performed || client.approveCalls != 1 || client.approveID != 123 {
		t.Fatalf("unexpected result=%+v client=%+v", result, client)
	}
}

func TestRunPRApproveCommandYesSkipsConfirmation(t *testing.T) {
	client := &fakeWriteClient{}

	result, err := runPRApproveCommand(authenticatedConfig(), []string{"123", "--yes"}, testWriteDeps(client, func(prompt string) bool {
		t.Fatalf("confirmation should not be called when --yes is provided")
		return false
	}))
	if err != nil {
		t.Fatalf("runPRApproveCommand returned error: %v", err)
	}
	if !result.Performed || client.approveCalls != 1 {
		t.Fatalf("expected approve to run once, got result=%+v calls=%d", result, client.approveCalls)
	}
}

func TestRunPRApproveCommandAbortDoesNotCallClient(t *testing.T) {
	client := &fakeWriteClient{}

	result, err := runPRApproveCommand(authenticatedConfig(), []string{"123"}, testWriteDeps(client, func(prompt string) bool {
		return false
	}))
	if err != nil {
		t.Fatalf("runPRApproveCommand returned error: %v", err)
	}
	if result.Performed || result.Message != "Aborted" {
		t.Fatalf("expected aborted result, got %+v", result)
	}
	if client.approveCalls != 0 {
		t.Fatalf("approve should not be called after abort, got %d calls", client.approveCalls)
	}
}

func TestRunPRMergeCommandParsesMessageAndYes(t *testing.T) {
	client := &fakeWriteClient{pr: PullRequest{ID: 123, Title: "Fix login", Destination: PRSide{Branch: BranchRef{Name: "main"}}}}

	result, err := runPRMergeCommand(authenticatedConfig(), []string{"123", "--message", "Ship it", "--yes"}, testWriteDeps(client, func(prompt string) bool {
		t.Fatalf("confirmation should not be called when --yes is provided")
		return false
	}))
	if err != nil {
		t.Fatalf("runPRMergeCommand returned error: %v", err)
	}
	if !result.Performed || client.prCalls != 1 || client.mergeCalls != 1 || client.mergeID != 123 || client.mergeMessage != "Ship it" {
		t.Fatalf("unexpected result=%+v client=%+v", result, client)
	}
}

func TestRunPRMergeCommandAbortDoesNotMerge(t *testing.T) {
	client := &fakeWriteClient{pr: PullRequest{ID: 123, Title: "Fix login", Destination: PRSide{Branch: BranchRef{Name: "main"}}}}

	result, err := runPRMergeCommand(authenticatedConfig(), []string{"123"}, testWriteDeps(client, func(prompt string) bool {
		if !strings.Contains(prompt, "Merge PR #123") {
			t.Fatalf("unexpected prompt: %s", prompt)
		}
		return false
	}))
	if err != nil {
		t.Fatalf("runPRMergeCommand returned error: %v", err)
	}
	if result.Performed || client.mergeCalls != 0 {
		t.Fatalf("expected merge to be aborted, got result=%+v calls=%d", result, client.mergeCalls)
	}
}

func TestRunPipelineRunCommandAsksForConfirmationByDefault(t *testing.T) {
	client := &fakeWriteClient{}
	confirmCalled := false

	result, err := runPipelineRunCommand(authenticatedConfig(), []string{"--branch", "main"}, testWriteDeps(client, func(prompt string) bool {
		confirmCalled = true
		if !strings.Contains(prompt, "Run pipeline for branch main") {
			t.Fatalf("unexpected prompt: %s", prompt)
		}
		return true
	}))
	if err != nil {
		t.Fatalf("runPipelineRunCommand returned error: %v", err)
	}
	if !confirmCalled {
		t.Fatal("expected confirmation to be requested")
	}
	if !result.Performed || client.pipelineCalls != 1 || client.pipelineBranch != "main" {
		t.Fatalf("unexpected result=%+v client=%+v", result, client)
	}
}

func TestRunPipelineRunCommandYesUsesCurrentBranchWhenMissing(t *testing.T) {
	client := &fakeWriteClient{}

	result, err := runPipelineRunCommand(authenticatedConfig(), []string{"--yes"}, testWriteDeps(client, func(prompt string) bool {
		t.Fatalf("confirmation should not be called when --yes is provided")
		return false
	}))
	if err != nil {
		t.Fatalf("runPipelineRunCommand returned error: %v", err)
	}
	if !result.Performed || client.pipelineCalls != 1 || client.pipelineBranch != "current-branch" {
		t.Fatalf("unexpected result=%+v client=%+v", result, client)
	}
}

func TestWriteCommandsRequireAuthentication(t *testing.T) {
	client := &fakeWriteClient{}
	_, err := runPRApproveCommand(Config{}, []string{"123", "--yes"}, testWriteDeps(client, nil))
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestWriteCommandsRejectMissingOrInvalidIDs(t *testing.T) {
	client := &fakeWriteClient{}

	_, err := runPRApproveCommand(authenticatedConfig(), []string{"--yes"}, testWriteDeps(client, nil))
	if err == nil || !strings.Contains(err.Error(), "missing numeric id") {
		t.Fatalf("expected missing id error, got %v", err)
	}

	_, err = runPRMergeCommand(authenticatedConfig(), []string{"not-a-number", "--yes"}, testWriteDeps(client, nil))
	if err == nil {
		t.Fatal("expected invalid numeric id error")
	}
}

func TestWriteCommandsReturnClientErrors(t *testing.T) {
	client := &fakeWriteClient{approveErr: errors.New("approval failed")}

	_, err := runPRApproveCommand(authenticatedConfig(), []string{"123", "--yes"}, testWriteDeps(client, nil))
	if err == nil || !strings.Contains(err.Error(), "approval failed") {
		t.Fatalf("expected client error, got %v", err)
	}
}
