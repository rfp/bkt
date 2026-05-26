package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "test-token-value"

func newTestClient(server *httptest.Server) *Client {
	return &Client{
		BaseURL: strings.TrimRight(server.URL, "/"),
		Email:   "rui@example.com",
		Token:   testToken,
		HTTP:    server.Client(),
	}
}

func requireBasicAuth(t *testing.T, r *http.Request) {
	t.Helper()
	username, password, ok := r.BasicAuth()
	if !ok {
		t.Fatal("expected Basic Auth header")
	}
	if username != "rui@example.com" {
		t.Fatalf("expected Basic Auth username %q, got %q", "rui@example.com", username)
	}
	if password != testToken {
		t.Fatalf("expected configured test token")
	}
}

func TestCurrentUserSendsAuthAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/user" {
			t.Fatalf("expected /user, got %s", r.URL.Path)
		}
		requireBasicAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"display_name":"Rui Patinha","nickname":"rfp","account_id":"abc123"}`))
	}))
	defer server.Close()

	user, err := newTestClient(server).CurrentUser()
	if err != nil {
		t.Fatalf("CurrentUser returned error: %v", err)
	}
	if user.DisplayName != "Rui Patinha" || user.Nickname != "rfp" || user.AccountID != "abc123" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestRepoDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/repositories/workspace/repo" {
			t.Fatalf("expected repository path, got %s", r.URL.Path)
		}
		requireBasicAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"repo","full_name":"workspace/repo","slug":"repo","scm":"git","is_private":true,"links":{"html":{"href":"https://bitbucket.org/workspace/repo"}}}`))
	}))
	defer server.Close()

	repo, err := newTestClient(server).Repo("workspace", "repo")
	if err != nil {
		t.Fatalf("Repo returned error: %v", err)
	}
	if repo.FullName != "workspace/repo" || repo.Slug != "repo" || !repo.IsPrivate {
		t.Fatalf("unexpected repo: %+v", repo)
	}
}

func TestListPRsFollowsValidPagination(t *testing.T) {
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/workspace/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		requireBasicAuth(t, r)
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[{"id":2,"title":"Second","state":"OPEN","source":{"branch":{"name":"feature/two"}},"destination":{"branch":{"name":"main"}}}]}`))
			return
		}
		next := server.URL + "/repositories/workspace/repo/pullrequests?page=2"
		_, _ = w.Write([]byte(`{"values":[{"id":1,"title":"First","state":"OPEN","source":{"branch":{"name":"feature/one"}},"destination":{"branch":{"name":"main"}}}],"next":` + quoteJSON(next) + `}`))
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	prs, err := newTestClient(server).ListPRs("workspace", "repo", "OPEN")
	if err != nil {
		t.Fatalf("ListPRs returned error: %v", err)
	}
	if len(prs) != 2 || prs[0].ID != 1 || prs[1].ID != 2 {
		t.Fatalf("unexpected PRs: %+v", prs)
	}
}

func TestListPRsRejectsUnsafePaginationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireBasicAuth(t, r)
		_, _ = w.Write([]byte(`{"values":[],"next":"https://example.com/repositories/workspace/repo/pullrequests?page=2"}`))
	}))
	defer server.Close()

	_, err := newTestClient(server).ListPRs("workspace", "repo", "OPEN")
	if err == nil {
		t.Fatal("expected unsafe pagination URL to be rejected")
	}
	if !strings.Contains(err.Error(), "outside configured API host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePRSendsExpectedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repositories/workspace/repo/pullrequests" {
			t.Fatalf("expected pullrequests path, got %s", r.URL.Path)
		}
		requireBasicAuth(t, r)

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("could not decode payload: %v", err)
		}
		if payload["title"] != "Fix login" || payload["description"] != "Adds validation" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		source := payload["source"].(map[string]any)["branch"].(map[string]any)["name"]
		destination := payload["destination"].(map[string]any)["branch"].(map[string]any)["name"]
		if source != "feature/login" || destination != "main" {
			t.Fatalf("unexpected branches: source=%v destination=%v", source, destination)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":123,"title":"Fix login","state":"OPEN"}`))
	}))
	defer server.Close()

	pr, err := newTestClient(server).CreatePR("workspace", "repo", "Fix login", "Adds validation", "feature/login", "main")
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if pr.ID != 123 || pr.Title != "Fix login" {
		t.Fatalf("unexpected PR: %+v", pr)
	}
}

func TestMergePRSendsExpectedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/repositories/workspace/repo/pullrequests/123/merge" {
			t.Fatalf("expected merge path, got %s", r.URL.Path)
		}
		requireBasicAuth(t, r)

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("could not decode payload: %v", err)
		}
		if payload["message"] != "Ship it" {
			t.Fatalf("unexpected merge message payload: %+v", payload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := newTestClient(server).MergePR("workspace", "repo", 123, "Ship it"); err != nil {
		t.Fatalf("MergePR returned error: %v", err)
	}
}

func TestClientFormatsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requireBasicAuth(t, r)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("api rejected " + testToken))
	}))
	defer server.Close()

	_, err := newTestClient(server).CurrentUser()
	if err == nil {
		t.Fatal("expected API error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("API error leaked token: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("expected redacted token in error, got: %v", err)
	}
}

func quoteJSON(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}
