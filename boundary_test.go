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
	"testing"
	"time"
)

func TestJSONErrorPreservesRedactedDetails(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"partial":"mock-secret"},"errors":[{"message":"echo mock-secret","extensions":{"code":"FORBIDDEN"}}]}`)
	}))
	defer s.Close()
	var out, errOut bytes.Buffer
	a := &app{home: t.TempDir(), getenv: func(k string) string {
		if k == "LINEAR_API_KEY" {
			return "mock-secret"
		}
		return ""
	}, in: strings.NewReader(""), out: &out, errOut: &errOut, endpoint: s.URL}
	if code := a.run(context.Background(), []string{"doctor", "--json"}); code != 1 {
		t.Fatal(code)
	}
	if out.Len() != 0 || strings.Contains(errOut.String(), "mock-secret") {
		t.Fatal("error data leaked", out.String(), errOut.String())
	}
	var payload object
	if e := json.Unmarshal(errOut.Bytes(), &payload); e != nil {
		t.Fatal(e)
	}
	details := payload["details"].(map[string]any)
	if details["data"].(map[string]any)["partial"] != "[REDACTED]" {
		t.Fatal(payload)
	}
}
func TestTeamMismatchAndZeroMatchesDoNotMutate(t *testing.T) {
	m := newMock(t)
	code, _, err := invoke(t, m, "", "update", "KNO-1", "--team-id", idB, "--title", "No")
	if code != 2 || !strings.Contains(err, "outside the selected team") {
		t.Fatal(code, err)
	}
	for _, q := range m.calls() {
		if strings.Contains(q.Query, "mutation") {
			t.Fatal("cross-team write")
		}
	}
	for _, tc := range []struct {
		marker string
		args   []string
	}{
		{"ResolveTeam", []string{"create", "--title", "No", "--team", "KNO"}},
		{"ResolveState", []string{"update", "KNO-1", "--state", "Missing"}},
		{"ResolveUser", []string{"update", "KNO-1", "--assignee", "missing@example.com"}},
	} {
		m := newMock(t)
		m.respond = func(q recordedRequest) object {
			r := defaultResponse(q)
			if strings.Contains(q.Query, tc.marker) {
				for k := range r {
					r[k] = object{"nodes": []any{}}
				}
			}
			return r
		}
		if code, _, _ := invoke(t, m, "", tc.args...); code != 2 {
			t.Fatal(tc.marker, code)
		}
	}
}
func TestGraphQLFilesAndContentBoundaries(t *testing.T) {
	m := newMock(t)
	dir := t.TempDir()
	qfile, vfile := filepath.Join(dir, "q.graphql"), filepath.Join(dir, "vars.json")
	os.WriteFile(qfile, []byte("query X($s: String!) { x(s: $s) }"), 0600)
	os.WriteFile(vfile, []byte(`{"s":"value"}`), 0600)
	mustRun(t, m, "", "graphql", "--query-file", qfile, "--variables-file", vfile)
	mustRun(t, m, `{"s":"stdin"}`, "graphql", "query X($s: String!) { x(s: $s) }", "--variables-file", "-")
	calls := m.calls()
	if calls[0].Variables["s"] != "value" || calls[1].Variables["s"] != "stdin" {
		t.Fatal(calls)
	}
	for _, tc := range []struct {
		stdin string
		args  []string
		want  int
	}{
		{"", []string{"graphql"}, 2},
		{"", []string{"graphql", "--query-file", qfile + ".missing"}, 1},
		{"", []string{"create", "--title", "X", "--team-id", idTeam, "--desc-file", vfile + ".missing"}, 1},
		{strings.Repeat("x", (8<<20)+1), []string{"comment", "KNO-1", "--body-file", "-"}, 2},
	} {
		if code, _, _ := invoke(t, m, tc.stdin, tc.args...); code != tc.want {
			t.Fatal(tc.args, code, tc.want)
		}
	}
}
func TestPageInfoFailures(t *testing.T) {
	for _, conn := range []object{{"nodes": []any{}}, {"nodes": []any{}, "pageInfo": object{}}, {"nodes": []any{}, "pageInfo": object{"hasNextPage": true, "endCursor": nil}}, {"pageInfo": object{"hasNextPage": false}}} {
		m := newMock(t)
		m.respond = func(recordedRequest) object { return object{"teams": conn} }
		if code, _, _ := invoke(t, m, "", "teams", "--all"); code != 1 {
			t.Fatal(conn, code)
		}
	}
}
func TestClientResponseCap(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
	}))
	defer s.Close()
	if _, e := newAPIClient(s.URL, "test-key", time.Second).do(context.Background(), "query { x }", nil); e == nil || !strings.Contains(e.Error(), "16 MiB") {
		t.Fatal(e)
	}
}
func TestLabelsInput(t *testing.T) {
	for _, s := range []string{idA + "," + idB, `["` + idA + `","` + idB + `"]`} {
		ids, e := parseLabelIDs(s)
		if e != nil || len(ids) != 2 {
			t.Fatal(ids, e)
		}
	}
	for _, s := range []string{"[null]", "not-uuid", "[", "[1]"} {
		if _, e := parseLabelIDs(s); e == nil {
			t.Fatal("invalid labels accepted", s)
		}
	}
}
