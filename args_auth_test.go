package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parsed(t *testing.T, args ...string) options {
	t.Helper()
	o, e := parseArgs(args)
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func noEnv(string) string { return "" }
func TestParseArgs(t *testing.T) {
	o := parsed(t, "--workspace=work", "update", "kno123", "--desc", "a\n\"b\" \\c", "--priority=0", "--json")
	if o.Command != "update" || o.Args[0] != "kno123" || o.get("desc") != "a\n\"b\" \\c" || o.get("workspace") != "work" || !o.has("json") {
		t.Fatalf("%+v", o)
	}
	o = parsed(t, "graphql", "--", "-literal")
	if o.Args[0] != "-literal" {
		t.Fatal(o)
	}
	o = parsed(t, "issues", "--all=false", "--description=x")
	if o.has("all") || o.get("desc") != "x" {
		t.Fatal(o)
	}
	for _, args := range [][]string{{"issue", "--wat"}, {"issue", "--first"}, {"issue", "--first", "--json"}, {"issue", "--all=oops"}, {"issue", "--team=A", "--team=B"}, {"issue", "--skills"}, {"issue", "--version"}} {
		if _, e := parseArgs(args); e == nil {
			t.Errorf("expected parse failure: %v", args)
		}
	}
}
func TestFlagsValidation(t *testing.T) {
	if e := parsed(t, "issues", "--body=x").validate("first"); e == nil {
		t.Fatal("unknown command-specific flag accepted")
	}
	if e := parsed(t, "issue").requireArgs(1); e == nil {
		t.Fatal("missing arg accepted")
	}
	for _, s := range []string{"0", "101", "nan", "-1"} {
		if _, e := parsed(t, "issues", "--first="+s).first(); e == nil {
			t.Fatal(s)
		}
	}
	if n, e := parsed(t, "issues").first(); e != nil || n != 25 {
		t.Fatal(n, e)
	}
	if n, e := parsed(t, "issues", "--first=100").first(); e != nil || n != 100 {
		t.Fatal(n, e)
	}
	if e := parsed(t, "archive").confirmed(); e == nil {
		t.Fatal("missing --yes accepted")
	}
	if e := parsed(t, "archive", "--dry-run").confirmed(); e != nil {
		t.Fatal(e)
	}
}
func writeToken(t *testing.T, home, alias, token string) string {
	t.Helper()
	p := filepath.Join(home, ".config", alias, "linear-token")
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(token), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestAuthPrecedenceAndIsolation(t *testing.T) {
	home := t.TempDir()
	p := writeToken(t, home, "work", "work-secret\n")
	writeToken(t, home, "personal", "personal-secret")
	env := map[string]string{"LINEAR_API_KEY": "env-secret", "LINEAR_WORKSPACE": "personal"}
	get := func(k string) string { return env[k] }
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--api-key", "flag-secret", "--token-file", p, "--workspace", "personal"}, "flag-secret"},
		{[]string{"--token-file", p, "--workspace", "personal"}, "work-secret"},
		{[]string{"--workspace", "personal"}, "personal-secret"},
		{nil, "env-secret"},
	}
	for _, tc := range cases {
		a, e := resolveAuth(parsed(t, append([]string{"doctor"}, tc.args...)...), home, get)
		if e != nil || a.Token != tc.want {
			t.Fatalf("%v got=%+v err=%v", tc.args, a, e)
		}
		if strings.Contains(a.Source, a.Token) {
			t.Fatal("token leaked in source")
		}
	}
	delete(env, "LINEAR_API_KEY")
	a, e := resolveAuth(parsed(t, "doctor"), home, get)
	if e != nil || a.Token != "personal-secret" {
		t.Fatal(a, e)
	}
	delete(env, "LINEAR_WORKSPACE")
	if _, e = resolveAuth(parsed(t, "doctor"), home, get); e == nil {
		t.Fatal("ambiguous aliases accepted")
	}
	for _, alias := range []string{"../personal", "/personal", "personal/..", "", ".", "a b", "missing"} {
		if _, e := resolveAuth(parsed(t, "doctor", "--workspace", alias), home, func(string) string { return "fallback-token" }); e == nil {
			t.Fatal("invalid explicit alias fell back", alias)
		}
	}
	if _, e := resolveAuth(parsed(t, "doctor", "--token-file", p+".missing"), home, get); e == nil {
		t.Fatal("missing explicit file accepted")
	}
	if _, e := resolveAuth(parsed(t, "doctor", "--api-key", ""), home, get); e == nil {
		t.Fatal("empty explicit key accepted")
	}
}
func TestAuthDiscovery(t *testing.T) {
	home := t.TempDir()
	if _, e := resolveAuth(parsed(t, "doctor"), home, noEnv); e == nil {
		t.Fatal("missing auth accepted")
	}
	writeToken(t, home, "only", "one-token")
	a, e := resolveAuth(parsed(t, "doctor"), home, noEnv)
	if e != nil || a.Token != "one-token" {
		t.Fatal(a, e)
	}
	p := writeToken(t, home, "blank", " \n")
	if _, e := readToken(p); e == nil {
		t.Fatal("empty token accepted")
	}
	if e := os.WriteFile(p, []byte("two\nlines"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readToken(p); e == nil {
		t.Fatal("multiline token accepted")
	}
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "xdg"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "xdg", "linear-token"), []byte("xdg-token"), 0600); e != nil {
		t.Fatal(e)
	}
	a, e = resolveAuth(parsed(t, "doctor", "--workspace", "xdg"), home, func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return root
		}
		return ""
	})
	if e != nil || a.Token != "xdg-token" {
		t.Fatal(a, e)
	}
	if expandHome("~/token", home) != filepath.Join(home, "token") {
		t.Fatal("home expansion")
	}
}
func TestNormalizeIssue(t *testing.T) {
	cases := []struct{ in, team, want string }{
		{"KNO-12", "", "KNO-12"}, {"kno-12", "", "KNO-12"}, {"kno12", "", "KNO-12"},
		{"https://linear.app/example/issue/KNO-12/title", "", "KNO-12"}, {"12", "KNO", "KNO-12"},
		{"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
	}
	for _, tc := range cases {
		got, e := normalizeIssue(tc.in, tc.team)
		if e != nil || got != tc.want {
			t.Error(tc, got, e)
		}
	}
	for _, ref := range []string{"12", "anything", "https://evil.test/x/issue/KNO-12", "https://linear.app/no-issue", "https://linear.app:444/x/issue/KNO-12", ""} {
		if _, e := normalizeIssue(ref, ""); e == nil {
			t.Fatal("invalid reference accepted", ref)
		}
	}
}
