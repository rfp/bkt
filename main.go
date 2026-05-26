package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	keyringService    = "bkt"
	defaultAPIBaseURL = "https://api.bitbucket.org/2.0"
	maxAPIErrorBody   = 500
)

var (
	keyringSet    = keyring.Set
	keyringGet    = keyring.Get
	keyringDelete = keyring.Delete
	inputLine     = readLine
	inputSecret   = readSecret
	newUserClient = func(cfg Config) (userClient, error) { return newClient(cfg) }
)

type userClient interface {
	CurrentUser() (User, error)
}

type Config struct {
	Email      string
	Username   string
	Token      string
	Workspace  string
	APIBaseURL string
}

type Client struct {
	BaseURL string
	Email   string
	Token   string
	HTTP    *http.Client
}

type Link struct{ Href string `json:"href"` }
type Links map[string]Link

type User struct {
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	AccountID   string `json:"account_id"`
}

type Repository struct {
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Slug      string `json:"slug"`
	SCM       string `json:"scm"`
	IsPrivate bool   `json:"is_private"`
	Links     Links  `json:"links"`
}

type BranchRef struct{ Name string `json:"name"` }

type PRSide struct {
	Branch     BranchRef  `json:"branch"`
	Repository Repository `json:"repository"`
}

type PullRequest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	Description string `json:"description"`
	Source      PRSide `json:"source"`
	Destination PRSide `json:"destination"`
	Author      User   `json:"author"`
	Links       Links  `json:"links"`
}

type PipelineTarget struct {
	RefName string `json:"ref_name"`
	Type    string `json:"type"`
}

type Pipeline struct {
	UUID        string         `json:"uuid"`
	BuildNumber int            `json:"build_number"`
	State       map[string]any `json:"state"`
	Target      PipelineTarget `json:"target"`
	CreatedOn   string         `json:"created_on"`
	CompletedOn string         `json:"completed_on"`
	Links       Links          `json:"links"`
}

type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

type RepoRef struct {
	Workspace string
	Slug      string
	RemoteURL string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	app := NewDefaultApp(cfg)
	if err := app.Run(os.Args[1:]); err != nil {
		fatal(err)
	}
}

func help() {
	fmt.Print(`bkt, a tiny Bitbucket Cloud CLI

Usage:
  bkt auth login|status|logout
  bkt repo view [--json]
  bkt pr list [--state OPEN] [--json]
  bkt pr view <id> [--json] [--web]
  bkt pr create [--title T] [--description D] [--source B] [--target main]
  bkt pr checkout <id>
  bkt pr approve <id> [--yes]
  bkt pr merge <id> [--message M] [--yes]
  bkt pipeline list [--json]
  bkt pipeline run [--branch B] [--json] [--yes]
`)
}

func auth(cfg Config, args []string) {
	if len(args) == 0 {
		fatal(errors.New("missing auth subcommand"))
	}

	switch args[0] {
	case "login":
		if err := authLogin(cfg); err != nil {
			fatal(err)
		}
	case "status":
		ensureAuth(cfg)
		u, err := clientOrFatal(cfg).CurrentUser()
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Logged in as %s\nEmail: %s\nUsername: %s\nAPI: %s\n", u.DisplayName, cfg.Email, cfg.Username, cfg.APIBaseURL)
	case "logout":
		if err := authLogout(cfg); err != nil {
			fatal(err)
		}
		fmt.Println("Logged out")
	default:
		fatal(fmt.Errorf("unknown auth subcommand: %s", args[0]))
	}
}

func authLogin(cfg Config) error {
	var err error
	cfg.APIBaseURL, err = validateAPIBaseURL(cfg.APIBaseURL)
	if err != nil {
		return err
	}

	cfg.Email = inputLine("Atlassian account email: ")
	cfg.Username = inputLine("Bitbucket username, optional: ")
	token, err := inputSecret("API token: ")
	if err != nil {
		return err
	}
	cfg.Token = token
	cfg.Workspace = inputLine("Default workspace, optional: ")

	if cfg.Email == "" || cfg.Token == "" {
		return errors.New("email and token are required")
	}

	client, err := newUserClient(cfg)
	if err != nil {
		return err
	}
	u, err := client.CurrentUser()
	if err != nil {
		return err
	}
	if cfg.Username == "" {
		cfg.Username = u.Nickname
	}
	if err := saveToken(cfg.Email, cfg.Token); err != nil {
		return fmt.Errorf("could not store API token in keychain: %w", err)
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s (%s)\n", u.DisplayName, cfg.Email)
	return nil
}

func authLogout(cfg Config) error {
	if cfg.Email != "" {
		if err := deleteToken(cfg.Email); err != nil {
			return fmt.Errorf("could not remove API token from keychain: %w", err)
		}
	}
	return deleteConfig()
}

func repo(cfg Config, args []string) {
	if len(args) == 0 || args[0] != "view" {
		fatal(errors.New("usage: bkt repo view [--json]"))
	}
	fs := flag.NewFlagSet("repo view", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args[1:])

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	r, err := clientOrFatal(cfg).Repo(rc.Workspace, rc.Slug)
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		printJSON(r)
		return
	}
	fmt.Printf("%s\nSCM: %s\nPrivate: %t\nURL: %s\n", r.FullName, r.SCM, r.IsPrivate, r.Links["html"].Href)
}

func pr(cfg Config, args []string) {
	if len(args) == 0 {
		fatal(errors.New("missing pr subcommand"))
	}

	switch args[0] {
	case "list":
		prList(cfg, args[1:])
	case "view":
		prView(cfg, args[1:])
	case "create":
		prCreate(cfg, args[1:])
	case "checkout":
		prCheckout(cfg, args[1:])
	case "approve":
		prApprove(cfg, args[1:])
	case "merge":
		prMerge(cfg, args[1:])
	default:
		fatal(fmt.Errorf("unknown pr subcommand: %s", args[0]))
	}
}

func prList(cfg Config, args []string) {
	fs := flag.NewFlagSet("pr list", flag.ExitOnError)
	state := fs.String("state", "OPEN", "OPEN, MERGED, DECLINED, SUPERSEDED")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	prs, err := clientOrFatal(cfg).ListPRs(rc.Workspace, rc.Slug, *state)
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		printJSON(prs)
		return
	}

	rows := [][]string{}
	for _, pr := range prs {
		rows = append(rows, []string{strconv.Itoa(pr.ID), pr.State, pr.Source.Branch.Name, pr.Destination.Branch.Name, pr.Title})
	}
	table([]string{"ID", "STATE", "SOURCE", "TARGET", "TITLE"}, rows)
}

func prView(cfg Config, args []string) {
	fs := flag.NewFlagSet("pr view", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "JSON output")
	web := fs.Bool("web", false, "open in browser")
	_ = fs.Parse(args)
	id := requireID(fs.Args())

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	pr, err := clientOrFatal(cfg).PR(rc.Workspace, rc.Slug, id)
	if err != nil {
		fatal(err)
	}
	if *web {
		openURL(pr.Links["html"].Href)
		return
	}
	if *jsonOut {
		printJSON(pr)
		return
	}
	fmt.Printf("#%d %s\nState: %s\nAuthor: %s\nSource: %s\nTarget: %s\nURL: %s\n\n%s\n", pr.ID, pr.Title, pr.State, pr.Author.DisplayName, pr.Source.Branch.Name, pr.Destination.Branch.Name, pr.Links["html"].Href, pr.Description)
}

func prCreate(cfg Config, args []string) {
	fs := flag.NewFlagSet("pr create", flag.ExitOnError)
	title := fs.String("title", "", "PR title")
	desc := fs.String("description", "", "PR description")
	source := fs.String("source", "", "source branch")
	target := fs.String("target", "main", "target branch")
	jsonOut := fs.Bool("json", false, "JSON output")
	_ = fs.Parse(args)

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	if *source == "" {
		*source, err = currentBranch()
		if err != nil {
			fatal(err)
		}
	}
	if *title == "" {
		*title = inputLine("Title: ")
	}
	if *desc == "" {
		*desc = inputLine("Description: ")
	}
	pr, err := clientOrFatal(cfg).CreatePR(rc.Workspace, rc.Slug, *title, *desc, *source, *target)
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		printJSON(pr)
		return
	}
	fmt.Printf("Created PR #%d: %s\n%s\n", pr.ID, pr.Title, pr.Links["html"].Href)
}

func prCheckout(cfg Config, args []string) {
	id := requireID(args)
	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	pr, err := clientOrFatal(cfg).PR(rc.Workspace, rc.Slug, id)
	if err != nil {
		fatal(err)
	}
	branch := pr.Source.Branch.Name
	if err := validateBranchName(branch); err != nil {
		fatal(err)
	}
	localBranch := localPRBranchName(id)
	fmt.Printf("Fetching %s and checking out %s\n", branch, localBranch)
	if err := run("git", "fetch", "origin", branch); err != nil {
		fatal(fmt.Errorf("git fetch failed: %w", err))
	}
	if err := run("git", "checkout", "-B", localBranch, "FETCH_HEAD"); err != nil {
		fatal(err)
	}
}

func prApprove(cfg Config, args []string) {
	fs := flag.NewFlagSet("pr approve", flag.ExitOnError)
	yes := fs.Bool("yes", false, "skip confirmation")
	_ = fs.Parse(args)
	id := requireID(fs.Args())

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	if !*yes && !confirmAction(fmt.Sprintf("Approve PR #%d in %s/%s?", id, rc.Workspace, rc.Slug)) {
		fmt.Println("Aborted")
		return
	}
	if err := clientOrFatal(cfg).ApprovePR(rc.Workspace, rc.Slug, id); err != nil {
		fatal(err)
	}
	fmt.Printf("Approved PR #%d\n", id)
}

func prMerge(cfg Config, args []string) {
	fs := flag.NewFlagSet("pr merge", flag.ExitOnError)
	msg := fs.String("message", "", "merge message")
	yes := fs.Bool("yes", false, "skip confirmation")
	_ = fs.Parse(args)
	id := requireID(fs.Args())

	ensureAuth(cfg)
	rc, err := detectRepo("origin")
	if err != nil {
		fatal(err)
	}
	client := clientOrFatal(cfg)
	pr, err := client.PR(rc.Workspace, rc.Slug, id)
	if err != nil {
		fatal(err)
	}
	if !*yes && !confirmAction(fmt.Sprintf("Merge PR #%d (%s) into %s in %s/%s?", id, pr.Title, pr.Destination.Branch.Name, rc.Workspace, rc.Slug)) {
		fmt.Println("Aborted")
		return
	}
	if err := client.MergePR(rc.Workspace, rc.Slug, id, *msg); err != nil {
		fatal(err)
	}
	fmt.Printf("Merged PR #%d\n", id)
}

func pipeline(cfg Config, args []string) {
	if len(args) == 0 {
		fatal(errors.New("missing pipeline subcommand"))
	}

	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("pipeline list", flag.ExitOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		_ = fs.Parse(args[1:])

		ensureAuth(cfg)
		rc, err := detectRepo("origin")
		if err != nil {
			fatal(err)
		}
		pipes, err := clientOrFatal(cfg).ListPipelines(rc.Workspace, rc.Slug)
		if err != nil {
			fatal(err)
		}
		if *jsonOut {
			printJSON(pipes)
			return
		}

		rows := [][]string{}
		for _, p := range pipes {
			rows = append(rows, []string{strconv.Itoa(p.BuildNumber), pipelineState(p.State), p.Target.RefName, p.CreatedOn})
		}
		table([]string{"BUILD", "STATE", "BRANCH", "CREATED"}, rows)
	case "run":
		fs := flag.NewFlagSet("pipeline run", flag.ExitOnError)
		branch := fs.String("branch", "", "branch to run")
		jsonOut := fs.Bool("json", false, "JSON output")
		yes := fs.Bool("yes", false, "skip confirmation")
		_ = fs.Parse(args[1:])

		ensureAuth(cfg)
		if *branch == "" {
			var err error
			*branch, err = currentBranch()
			if err != nil {
				fatal(err)
			}
		}
		rc, err := detectRepo("origin")
		if err != nil {
			fatal(err)
		}
		if !*yes && !confirmAction(fmt.Sprintf("Run pipeline for branch %s in %s/%s?", *branch, rc.Workspace, rc.Slug)) {
			fmt.Println("Aborted")
			return
		}
		p, err := clientOrFatal(cfg).RunPipeline(rc.Workspace, rc.Slug, *branch)
		if err != nil {
			fatal(err)
		}
		if *jsonOut {
			printJSON(p)
			return
		}
		fmt.Printf("Started pipeline #%d for branch %s\n", p.BuildNumber, *branch)
	default:
		fatal(fmt.Errorf("unknown pipeline subcommand: %s", args[0]))
	}
}

func clientOrFatal(cfg Config) *Client {
	client, err := newClient(cfg)
	if err != nil {
		fatal(err)
	}
	return client
}

func newClient(cfg Config) (*Client, error) {
	baseURL, err := validateAPIBaseURL(cfg.APIBaseURL)
	if err != nil {
		return nil, err
	}
	return &Client{BaseURL: baseURL, Email: cfg.Email, Token: cfg.Token, HTTP: &http.Client{Timeout: 30 * time.Second}}, nil
}

func validateAPIBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultAPIBaseURL
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid API base URL: %q", raw)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid API base URL %q: only https is allowed", raw)
	}
	if parsed.Host != "api.bitbucket.org" {
		return "", fmt.Errorf("invalid API base URL %q: only api.bitbucket.org is supported", raw)
	}

	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path != "/2.0" {
		return "", fmt.Errorf("invalid API base URL %q: expected path /2.0", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid API base URL %q: query strings and fragments are not allowed", raw)
	}

	return defaultAPIBaseURL, nil
}

func validateBranchName(branch string) error {
	if branch == "" {
		return errors.New("unsafe branch name: branch name is empty")
	}
	if branch != strings.TrimSpace(branch) {
		return fmt.Errorf("unsafe branch name %q: leading or trailing whitespace is not allowed", branch)
	}
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("unsafe branch name %q: branch names starting with '-' are not allowed", branch)
	}
	if strings.ContainsAny(branch, "\x00\n\r") {
		return fmt.Errorf("unsafe branch name %q: control characters are not allowed", branch)
	}
	return nil
}

func localPRBranchName(id int) string {
	return fmt.Sprintf("pr/%d", id)
}

func (c *Client) requestURL(path string) (string, error) {
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		return c.BaseURL + path, nil
	}

	requestURL, err := url.Parse(path)
	if err != nil || requestURL.Scheme == "" || requestURL.Host == "" {
		return "", fmt.Errorf("invalid request URL: %q", path)
	}

	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}

	basePath := strings.TrimRight(baseURL.EscapedPath(), "/")
	requestPath := strings.TrimRight(requestURL.EscapedPath(), "/")

	if requestURL.Scheme != baseURL.Scheme || requestURL.Host != baseURL.Host {
		return "", fmt.Errorf("refusing to call URL outside configured API host: %q", path)
	}
	if requestPath != basePath && !strings.HasPrefix(requestPath, basePath+"/") {
		return "", fmt.Errorf("refusing to call URL outside configured API path: %q", path)
	}
	if requestURL.Fragment != "" {
		return "", fmt.Errorf("refusing to call URL with fragment: %q", path)
	}

	return requestURL.String(), nil
}

func (c *Client) do(method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}

	u, err := c.requestURL(path)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Email != "" && c.Token != "" {
		req.SetBasicAuth(c.Email, c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return formatAPIError(resp.StatusCode, data, c.Token)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func formatAPIError(statusCode int, body []byte, token string) error {
	message := strings.TrimSpace(string(body))
	if token != "" {
		message = strings.ReplaceAll(message, token, "[REDACTED]")
	}
	if len(message) > maxAPIErrorBody {
		message = message[:maxAPIErrorBody] + "..."
	}
	if message == "" {
		return fmt.Errorf("bitbucket API error %d", statusCode)
	}
	return fmt.Errorf("bitbucket API error %d: %s", statusCode, message)
}

func (c *Client) CurrentUser() (User, error) {
	var u User
	err := c.do(http.MethodGet, "/user", nil, &u)
	return u, err
}

func (c *Client) Repo(workspace, repo string) (Repository, error) {
	var r Repository
	err := c.do(http.MethodGet, "/repositories/"+url.PathEscape(workspace)+"/"+url.PathEscape(repo), nil, &r)
	return r, err
}

func (c *Client) ListPRs(workspace, repo, state string) ([]PullRequest, error) {
	path := "/repositories/" + url.PathEscape(workspace) + "/" + url.PathEscape(repo) + "/pullrequests?pagelen=50"
	if state != "" {
		path += "&state=" + url.QueryEscape(strings.ToUpper(state))
	}

	var all []PullRequest
	for path != "" {
		var p page[PullRequest]
		if err := c.do(http.MethodGet, path, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Values...)
		path = p.Next
	}
	return all, nil
}

func (c *Client) PR(workspace, repo string, id int) (PullRequest, error) {
	var pr PullRequest
	err := c.do(http.MethodGet, fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", url.PathEscape(workspace), url.PathEscape(repo), id), nil, &pr)
	return pr, err
}

func (c *Client) CreatePR(workspace, repo, title, description, source, target string) (PullRequest, error) {
	payload := map[string]any{
		"title":       title,
		"description": description,
		"source":      map[string]any{"branch": map[string]string{"name": source}},
		"destination": map[string]any{"branch": map[string]string{"name": target}},
	}
	var pr PullRequest
	err := c.do(http.MethodPost, "/repositories/"+url.PathEscape(workspace)+"/"+url.PathEscape(repo)+"/pullrequests", payload, &pr)
	return pr, err
}

func (c *Client) ApprovePR(workspace, repo string, id int) error {
	return c.do(http.MethodPost, fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/approve", url.PathEscape(workspace), url.PathEscape(repo), id), nil, nil)
}

func (c *Client) MergePR(workspace, repo string, id int, message string) error {
	payload := map[string]any{}
	if message != "" {
		payload["message"] = message
	}
	return c.do(http.MethodPost, fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/merge", url.PathEscape(workspace), url.PathEscape(repo), id), payload, nil)
}

func (c *Client) ListPipelines(workspace, repo string) ([]Pipeline, error) {
	path := "/repositories/" + url.PathEscape(workspace) + "/" + url.PathEscape(repo) + "/pipelines/?pagelen=20"
	var all []Pipeline
	for path != "" {
		var p page[Pipeline]
		if err := c.do(http.MethodGet, path, nil, &p); err != nil {
			return nil, err
		}
		all = append(all, p.Values...)
		path = p.Next
	}
	return all, nil
}

func (c *Client) RunPipeline(workspace, repo, branch string) (Pipeline, error) {
	payload := map[string]any{"target": map[string]any{"type": "pipeline_ref_target", "ref_type": "branch", "ref_name": branch}}
	var p Pipeline
	err := c.do(http.MethodPost, "/repositories/"+url.PathEscape(workspace)+"/"+url.PathEscape(repo)+"/pipelines/", payload, &p)
	return p, err
}

func detectRepo(remote string) (RepoRef, error) {
	u, err := remoteURL(remote)
	if err != nil {
		return RepoRef{}, err
	}
	res := []*regexp.Regexp{
		regexp.MustCompile(`bitbucket\.org[:/]([^/]+)/([^/.]+)(?:\.git)?$`),
		regexp.MustCompile(`bitbucket\.org[:/]([^/]+)/(.+?)(?:\.git)?$`),
	}
	for _, re := range res {
		m := re.FindStringSubmatch(u)
		if len(m) == 3 {
			return RepoRef{Workspace: m[1], Slug: strings.TrimSuffix(m[2], ".git"), RemoteURL: u}, nil
		}
	}
	return RepoRef{}, errors.New("remote does not look like a Bitbucket Cloud URL")
}

func remoteURL(remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	out, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		return "", errors.New("could not read git remote; are you inside a git repository?")
	}
	return strings.TrimSpace(string(out)), nil
}

func currentBranch() (string, error) {
	out, err := exec.Command("git", "branch", "--show-current").Output()
	if err != nil {
		return "", err
	}
	b := strings.TrimSpace(string(out))
	if b == "" {
		return "", errors.New("could not detect current git branch")
	}
	return b, nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func configPath() (string, error) {
	if v := os.Getenv("BKT_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "config"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "bkt", "config"), nil
}

func loadConfig() (Config, error) {
	cfg := Config{APIBaseURL: defaultAPIBaseURL}
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	legacyToken := ""
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch k {
		case "email":
			cfg.Email = v
		case "username":
			cfg.Username = v
		case "token":
			legacyToken = v
		case "workspace":
			cfg.Workspace = v
		case "api_base_url":
			cfg.APIBaseURL = v
		}
	}
	if err := s.Err(); err != nil {
		return cfg, err
	}

	cfg.APIBaseURL, err = validateAPIBaseURL(cfg.APIBaseURL)
	if err != nil {
		return cfg, err
	}
	if cfg.Email == "" {
		return cfg, nil
	}

	token, err := loadToken(cfg.Email)
	if err == nil {
		cfg.Token = token
		return cfg, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return cfg, fmt.Errorf("could not read API token from keychain: %w", err)
	}
	if legacyToken != "" {
		if err := saveToken(cfg.Email, legacyToken); err != nil {
			return cfg, fmt.Errorf("could not migrate API token to keychain: %w", err)
		}
		cfg.Token = legacyToken
		if err := saveConfig(cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	var err error
	cfg.APIBaseURL, err = validateAPIBaseURL(cfg.APIBaseURL)
	if err != nil {
		return err
	}
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	content := fmt.Sprintf("email=%s\nusername=%s\nworkspace=%s\napi_base_url=%s\n", cfg.Email, cfg.Username, cfg.Workspace, cfg.APIBaseURL)
	return os.WriteFile(p, []byte(content), 0600)
}

func deleteConfig() error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveToken(account, token string) error {
	return keyringSet(keyringService, account, token)
}

func loadToken(account string) (string, error) {
	return keyringGet(keyringService, account)
}

func deleteToken(account string) error {
	if err := keyringDelete(keyringService, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(b))
}

func table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, c := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, c)
		}
		fmt.Fprintln(w)
	}
	_ = w.Flush()
}

func pipelineState(s map[string]any) string {
	if v, ok := s["name"].(string); ok {
		return v
	}
	if v, ok := s["result"].(map[string]any); ok {
		if n, ok := v["name"].(string); ok {
			return n
		}
	}
	if v, ok := s["stage"].(map[string]any); ok {
		if n, ok := v["name"].(string); ok {
			return n
		}
	}
	return "UNKNOWN"
}

func requireID(args []string) int {
	if len(args) != 1 {
		fatal(errors.New("missing numeric id"))
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		fatal(err)
	}
	return id
}

func ensureAuth(cfg Config) {
	if cfg.Email == "" || cfg.Token == "" {
		fatal(errors.New("not authenticated; run: bkt auth login"))
	}
}

func confirmAction(prompt string) bool {
	answer := strings.ToLower(inputLine(prompt + " [y/N] "))
	return answer == "y" || answer == "yes"
}

func readLine(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func readSecret(prompt string) (string, error) {
	fmt.Print(prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(secret)), nil
}

func openURL(u string) {
	switch {
	case commandExists("open"):
		_ = run("open", u)
	case commandExists("xdg-open"):
		_ = run("xdg-open", u)
	default:
		fmt.Println(u)
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "bkt:", err)
	os.Exit(1)
}
