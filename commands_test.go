package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	idA       = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	idB       = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	idTeam    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	idState   = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	idUser    = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	idComment = "ffffffff-ffff-4fff-8fff-ffffffffffff"
)

type recordedRequest struct {
	Query     string `json:"query"`
	Variables object `json:"variables"`
}
type mockAPI struct {
	mu       sync.Mutex
	requests []recordedRequest
	respond  func(recordedRequest) object
	server   *httptest.Server
}

func newMock(t *testing.T) *mockAPI {
	t.Helper()
	m := &mockAPI{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q recordedRequest
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		m.mu.Lock()
		m.requests = append(m.requests, q)
		m.mu.Unlock()
		result := defaultResponse(q)
		if m.respond != nil {
			result = m.respond(q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(object{"data": result})
	}))
	t.Cleanup(m.server.Close)
	return m
}
func (m *mockAPI) calls() []recordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recordedRequest{}, m.requests...)
}
func fixtureIssue(ref string) object {
	id, key := idA, "KNO-1"
	if ref == idB || ref == "KNO-2" {
		id, key = idB, "KNO-2"
	}
	return object{"id": id, "identifier": key, "title": "Fixture", "team": object{"id": idTeam, "key": "KNO"}, "archivedAt": nil}
}
func page(nodes []any, next bool, cursor any) object {
	return object{"nodes": nodes, "pageInfo": object{"hasNextPage": next, "endCursor": cursor}}
}
func defaultResponse(q recordedRequest) object {
	switch {
	case strings.Contains(q.Query, "query Identity"):
		return object{"viewer": object{"id": idUser}, "organization": object{"name": "Test"}}
	case strings.Contains(q.Query, "query ResolveTeam"):
		return object{"teams": object{"nodes": []any{object{"id": idTeam, "key": "KNO"}}}}
	case strings.Contains(q.Query, "query ResolveState"):
		return object{"workflowStates": object{"nodes": []any{object{"id": idState, "name": "In Progress"}}}}
	case strings.Contains(q.Query, "query ResolveUser"):
		return object{"users": object{"nodes": []any{object{"id": idUser, "email": "someone@example.com"}}}}
	case strings.Contains(q.Query, "mutation"):
		for _, root := range []string{"issueCreate", "issueUpdate", "commentCreate", "commentUpdate", "commentDelete", "issueRelationCreate", "issueRelationDelete", "issueArchive", "issueUnarchive"} {
			if strings.Contains(q.Query, root+"(") {
				return object{root: object{"success": true, "issue": fixtureIssue(idA), "comment": object{"id": idComment, "body": "test"}, "issueRelation": object{"id": idComment}}}
			}
		}
	case strings.Contains(q.Query, "query Comments"):
		return object{"issue": object{"comments": page([]any{object{"id": idComment}}, false, nil)}}
	case strings.Contains(q.Query, "query ResolveIssue") || strings.Contains(q.Query, "query Issue("):
		return object{"issue": fixtureIssue(stringField(q.Variables, "id"))}
	default:
		for _, root := range []string{"issues", "searchIssues", "teams", "users", "workflowStates", "issueLabels", "projects", "cycles"} {
			if strings.Contains(q.Query, root+"(") {
				return object{root: page([]any{object{"id": idA}}, false, nil)}
			}
		}
	}
	return object{"ok": true}
}
func invoke(t *testing.T, m *mockAPI, input string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	a := &app{home: t.TempDir(), getenv: func(k string) string {
		if k == "LINEAR_API_KEY" {
			return "mock-secret"
		}
		return ""
	}, in: strings.NewReader(input), out: &out, errOut: &errOut, endpoint: m.server.URL}
	code := a.run(context.Background(), args)
	return code, out.String(), errOut.String()
}
func mustRun(t *testing.T, m *mockAPI, input string, args ...string) object {
	t.Helper()
	code, out, err := invoke(t, m, input, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d stderr=%s", args, code, err)
	}
	var result object
	if e := json.Unmarshal([]byte(out), &result); e != nil {
		t.Fatal(out, e)
	}
	return result
}
func TestReadCommands(t *testing.T) {
	for _, args := range [][]string{{"doctor"}, {"issue", "kno1", "--with-comments"}, {"comments", "KNO-1", "--all"}, {"teams"}, {"users", "--name", "person", "--email", "example.com"}, {"states", "--team", "KNO"}, {"labels", "--team", "KNO"}, {"projects", "--team", "KNO"}, {"cycles", "--team", "KNO"}, {"issues", "--team", "KNO", "--state", "In Progress", "--assignee", "none", "--project-id", "none"}, {"search", "a \"quoted\" search", "--team", "KNO"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) { m := newMock(t); mustRun(t, m, "", append(args, "--json")...) })
	}
}
func TestCreateAndUpdateVariables(t *testing.T) {
	m := newMock(t)
	text := "Unicode 日本語\nquotes \" backticks ` backslash \\ trailing\n\n"
	mustRun(t, m, "", "create", "--team-id", idTeam, "--title", "Title", "--desc", text, "--priority", "0", "--label-ids", idState+","+idUser)
	calls := m.calls()
	input := calls[len(calls)-1].Variables["input"].(map[string]any)
	if input["description"] != text || input["teamId"] != idTeam || input["priority"] != float64(0) {
		t.Fatal(input)
	}
	if strings.Contains(calls[0].Query, text) {
		t.Fatal("interpolated user content")
	}
	mustRun(t, m, "", "update", "kno1", "--state", "In Progress", "--assignee", "someone@example.com", "--parent-id", "KNO-2", "--due-date", "2026-10-20")
	calls = m.calls()
	last := calls[len(calls)-1]
	input = last.Variables["input"].(map[string]any)
	if last.Variables["id"] != idA || input["stateId"] != idState || input["assigneeId"] != idUser || input["parentId"] != idB {
		t.Fatal(last)
	}
	mustRun(t, m, "", "update", "KNO-1", "--desc=", "--label-ids=", "--assignee-id", "none", "--project-id", "none", "--cycle-id", "none", "--parent-id", "none", "--due-date", "none")
	calls = m.calls()
	input = calls[len(calls)-1].Variables["input"].(map[string]any)
	if input["description"] != "" || len(input["labelIds"].([]any)) != 0 {
		t.Fatal(input)
	}
	for _, key := range []string{"assigneeId", "projectId", "cycleId", "parentId", "dueDate"} {
		if v, ok := input[key]; !ok || v != nil {
			t.Fatal("field not explicitly cleared", key, input)
		}
	}
}
func TestFilesAndStdinPreserveText(t *testing.T) {
	m := newMock(t)
	text := "Line 1\n\nLine 3 with \"quotes\" \\ and 日本語\n"
	p := filepath.Join(t.TempDir(), "description.md")
	if e := os.WriteFile(p, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	mustRun(t, m, "", "create", "--title", "From file", "--team-id", idTeam, "--desc-file", p)
	calls := m.calls()
	if calls[len(calls)-1].Variables["input"].(map[string]any)["description"] != text {
		t.Fatal("file corrupted")
	}
	mustRun(t, m, text, "comment", "KNO-1", "--body-file", "-")
	calls = m.calls()
	if calls[len(calls)-1].Variables["input"].(map[string]any)["body"] != text {
		t.Fatal("stdin corrupted")
	}
	mustRun(t, m, text, "update-comment", idComment, "--body-file", "-")
	mustRun(t, m, "", "delete-comment", idComment, "--yes")
}
func TestDryRunAndConfirmation(t *testing.T) {
	m := newMock(t)
	result := mustRun(t, m, "", "create", "--title", "Dry run", "--team-id", idTeam, "--dry-run")
	if result["dryRun"] != true || len(m.calls()) != 0 {
		t.Fatal("dry run made request")
	}
	mustRun(t, m, "", "update", "KNO-1", "--title", "New", "--dry-run")
	mustRun(t, m, "", "comment", "KNO-1", "--body", "Dry", "--dry-run")
	mustRun(t, m, "", "archive", "KNO-1", "--dry-run")
	mustRun(t, m, "", "unarchive", "KNO-1", "--dry-run")
	mustRun(t, m, "", "delete-comment", idComment, "--dry-run")
	mustRun(t, m, "", "unrelate", idComment, "--dry-run")
	for _, q := range m.calls() {
		if strings.Contains(q.Query, "mutation") {
			t.Fatal("dry run mutated", q)
		}
	}
	for _, args := range [][]string{{"archive", "KNO-1"}, {"unarchive", "KNO-1"}, {"delete-comment", idComment}, {"unrelate", idComment}} {
		code, _, _ := invoke(t, m, "", args...)
		if code != 2 {
			t.Fatal("missing confirmation accepted", args, code)
		}
	}
	mustRun(t, m, "", "archive", "KNO-1", "--yes")
	mustRun(t, m, "", "unarchive", "KNO-1", "--yes")
	mustRun(t, m, "", "unrelate", idComment, "--yes")
}
func TestRelations(t *testing.T) {
	for _, tc := range []struct {
		kind, left, right string
		parent            bool
	}{
		{"blocks", idA, idB, false}, {"blockedBy", idB, idA, false}, {"related", idA, idB, false}, {"duplicate", idA, idB, false},
		{"parent", idB, idA, true}, {"child", idA, idB, true},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			m := newMock(t)
			mustRun(t, m, "", "relate", "KNO-1", "KNO-2", "--type", tc.kind)
			calls := m.calls()
			last := calls[len(calls)-1]
			in := last.Variables["input"].(map[string]any)
			if tc.parent {
				if last.Variables["id"] != tc.left || in["parentId"] != tc.right {
					t.Fatal(last)
				}
			} else {
				if in["issueId"] != tc.left || in["relatedIssueId"] != tc.right {
					t.Fatal(last)
				}
			}
		})
	}
}
func TestMutationFailuresAndAmbiguity(t *testing.T) {
	m := newMock(t)
	m.respond = func(q recordedRequest) object {
		if strings.Contains(q.Query, "mutation") {
			return object{"issueCreate": object{"success": false}}
		}
		return defaultResponse(q)
	}
	code, _, stderr := invoke(t, m, "", "create", "--title", "Fail", "--team-id", idTeam)
	if code != 1 || !strings.Contains(stderr, "success=false") {
		t.Fatal(code, stderr)
	}
	for _, resolver := range []string{"ResolveTeam", "ResolveState", "ResolveUser"} {
		t.Run(resolver, func(t *testing.T) {
			m := newMock(t)
			m.respond = func(q recordedRequest) object {
				r := defaultResponse(q)
				if strings.Contains(q.Query, resolver) {
					for k := range r {
						r[k] = object{"nodes": []any{object{"id": idA}, object{"id": idB}}}
					}
				}
				return r
			}
			args := []string{"create", "--title", "Ambiguous", "--team", "KNO"}
			if resolver == "ResolveState" {
				args = []string{"update", "KNO-1", "--state", "In Progress"}
			}
			if resolver == "ResolveUser" {
				args = []string{"update", "KNO-1", "--assignee", "someone@example.com"}
			}
			code, _, _ := invoke(t, m, "", args...)
			if code != 2 {
				t.Fatal("ambiguous resolver accepted", code)
			}
			for _, q := range m.calls() {
				if strings.Contains(q.Query, "mutation") {
					t.Fatal("mutation after ambiguous lookup")
				}
			}
		})
	}
}
func TestPagination(t *testing.T) {
	for _, command := range []string{"teams", "comments"} {
		t.Run(command, func(t *testing.T) {
			m := newMock(t)
			m.respond = func(q recordedRequest) object {
				cursor, _ := q.Variables["after"].(string)
				conn := page([]any{object{"id": "first"}}, true, "next")
				if cursor == "next" {
					conn = page([]any{object{"id": "second"}}, false, nil)
				}
				if command == "comments" {
					return object{"issue": object{"comments": conn}}
				}
				return object{"teams": conn}
			}
			args := []string{command}
			if command == "comments" {
				args = append(args, "KNO-1")
			}
			args = append(args, "--all", "--first", "1")
			out := mustRun(t, m, "", args...)
			if len(out["nodes"].([]any)) != 2 || len(m.calls()) != 2 {
				t.Fatal(out, m.calls())
			}
		})
	}
	m := newMock(t)
	m.respond = func(recordedRequest) object { return object{"teams": page([]any{}, true, "same")} }
	code, _, err := invoke(t, m, "", "teams", "--all")
	if code != 1 || !strings.Contains(err, "did not advance") || len(m.calls()) != 2 {
		t.Fatal(code, err)
	}
}
func TestRawGraphQL(t *testing.T) {
	m := newMock(t)
	query := "query Test($text: String!) { field(text: $text) }"
	mustRun(t, m, query, "graphql", "--variables", `{"text":"quotes \" and newline\n"}`)
	calls := m.calls()
	if calls[0].Query != query || calls[0].Variables["text"] != "quotes \" and newline\n" {
		t.Fatal(calls)
	}
	out := mustRun(t, m, "", "graphql", "mutation { anything }", "--dry-run")
	if out["dryRun"] != true || len(m.calls()) != 1 {
		t.Fatal("raw dry-run sent request")
	}
	for _, args := range [][]string{{"graphql", "--query-file", "-", "--variables-file", "-"}, {"graphql", "query{}", "--query-file", "x"}, {"graphql", "query{}", "--variables", "[]"}, {"graphql", "query{}", "--variables", "null"}} {
		if code, _, _ := invoke(t, m, "", args...); code != 2 {
			t.Fatal(args, code)
		}
	}
}
func TestInputValidationBeforeMutations(t *testing.T) {
	invalid := [][]string{
		{"create", "--team-id", idTeam}, {"create", "--team-id", idTeam, "--title", " "},
		{"create", "--team-id", idTeam, "--title", "X", "--priority", "5"},
		{"update", "KNO-1"}, {"update", "KNO-1", "--due-date", "not-date"},
		{"update", "KNO-1", "--state", "A", "--state-id", idState},
		{"update", "KNO-1", "--assignee", "A", "--assignee-id", idUser},
		{"update", "KNO-1", "--label-ids", "bad"}, {"update", "KNO-1", "--parent-id", "KNO-1"},
		{"update", "KNO-1", "--desc", "a", "--desc-file", "x"},
		{"comment", "KNO-1", "--body", " "}, {"comment", "KNO-1"},
		{"update-comment", "short", "--body", "x"}, {"delete-comment", "short", "--yes"},
		{"relate", "KNO-1", "KNO-2", "--type", "invalid"}, {"relate", "KNO-1", "KNO-1"},
		{"unrelate", "short", "--yes"}, {"states"}, {"cycles"},
		{"issues", "--first", "1000"}, {"issues", "--state", "A", "--state-id", idState},
	}
	for _, args := range invalid {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			m := newMock(t)
			code, _, _ := invoke(t, m, "", args...)
			if code != 2 {
				t.Fatal("expected usage error", code)
			}
			for _, q := range m.calls() {
				if strings.Contains(q.Query, "mutation") {
					t.Fatal("invalid input mutated")
				}
			}
		})
	}
}
func TestOfflineHelpSkillsAndExitCodes(t *testing.T) {
	m := newMock(t)
	for _, args := range [][]string{nil, {"--help"}, {"issue", "--help"}, {"help", "create"}, {"--skills"}, {"--skill"}, {"skill"}, {"--version"}, {"version"}, {"workspaces"}, {"accounts", "list"}} {
		code, out, err := invoke(t, m, "", args...)
		if code != 0 || out == "" {
			t.Fatal(args, code, out, err)
		}
	}
	if len(m.calls()) != 0 {
		t.Fatal("offline command sent HTTP")
	}
	for _, args := range [][]string{{"unknown"}, {"help", "unknown"}, {"accounts", "add"}, {"issue", "--wat"}} {
		if code, _, _ := invoke(t, m, "", args...); code != 2 {
			t.Fatal(args, code)
		}
	}
	var out, errOut bytes.Buffer
	a := &app{home: t.TempDir(), getenv: noEnv, in: strings.NewReader(""), out: &out, errOut: &errOut, endpoint: m.server.URL}
	if code := a.run(context.Background(), []string{"doctor", "--json"}); code != 3 {
		t.Fatal(code)
	}
	if out.Len() != 0 || !json.Valid(errOut.Bytes()) {
		t.Fatal("error output is not clean JSON", out.String(), errOut.String())
	}
	a = &app{home: t.TempDir(), getenv: func(k string) string {
		if k == "LINEAR_API_KEY" {
			return "secret"
		}
		return ""
	}, in: strings.NewReader(""), out: io.Discard, errOut: io.Discard, endpoint: m.server.URL}
	if code := a.run(context.Background(), []string{"doctor", "--timeout", "bad"}); code != 2 {
		t.Fatal(code)
	}
}
