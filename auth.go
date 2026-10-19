package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type authInfo struct{ Token, Source string }
type workspaceInfo struct {
	Alias     string `json:"alias"`
	TokenFile string `json:"tokenFile"`
}

var aliasRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func configRoot(home string, getenv func(string) string) string {
	if s := getenv("XDG_CONFIG_HOME"); s != "" {
		return s
	}
	return filepath.Join(home, ".config")
}
func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
func discoverWorkspaces(root string) ([]workspaceInfo, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []workspaceInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config directory: %w", err)
	}
	out := []workspaceInfo{}
	for _, e := range entries {
		if !aliasRE.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(root, e.Name(), "linear-token")
		fi, err := os.Stat(p)
		if err == nil && fi.Mode().IsRegular() {
			out = append(out, workspaceInfo{e.Name(), p})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, nil
}
func readToken(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", &cliError{3, fmt.Sprintf("cannot read credential file %s: %v", p, err)}
	}
	token := strings.TrimSpace(string(b))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return "", &cliError{3, "credential file must contain one nonempty token line"}
	}
	return token, nil
}
func aliasAuth(alias, root string) (authInfo, error) {
	if !aliasRE.MatchString(alias) {
		return authInfo{}, &cliError{3, "invalid workspace alias; use letters, digits, underscore or dash (no paths)"}
	}
	p := filepath.Join(root, alias, "linear-token")
	token, err := readToken(p)
	if err != nil {
		return authInfo{}, err
	}
	return authInfo{token, "workspace:" + alias + " (" + p + ")"}, nil
}
func resolveAuth(o options, home string, getenv func(string) string) (authInfo, error) {
	if o.supplied("api-key") {
		token := strings.TrimSpace(o.get("api-key"))
		if token == "" || strings.ContainsAny(token, "\r\n") {
			return authInfo{}, &cliError{3, "--api-key must be one nonempty token line"}
		}
		return authInfo{token, "flag:--api-key"}, nil
	}
	if o.supplied("token-file") {
		p := expandHome(o.get("token-file"), home)
		token, err := readToken(p)
		return authInfo{token, "token-file:" + p}, err
	}
	root := configRoot(home, getenv)
	if o.supplied("workspace") {
		return aliasAuth(o.get("workspace"), root)
	}
	if token := strings.TrimSpace(getenv("LINEAR_API_KEY")); token != "" {
		if strings.ContainsAny(token, "\r\n") {
			return authInfo{}, &cliError{3, "LINEAR_API_KEY must contain one token line"}
		}
		return authInfo{token, "env:LINEAR_API_KEY"}, nil
	}
	if alias := getenv("LINEAR_WORKSPACE"); alias != "" {
		return aliasAuth(alias, root)
	}
	aliases, err := discoverWorkspaces(root)
	if err != nil {
		return authInfo{}, &cliError{3, err.Error()}
	}
	if len(aliases) == 1 {
		return aliasAuth(aliases[0].Alias, root)
	}
	if len(aliases) > 1 {
		names := []string{}
		for _, w := range aliases {
			names = append(names, w.Alias)
		}
		return authInfo{}, &cliError{3, "multiple workspaces found (" + strings.Join(names, ", ") + "); choose --workspace ALIAS or export LINEAR_API_KEY"}
	}
	return authInfo{}, &cliError{3, "no Linear token; use --workspace ALIAS, --token-file PATH, or LINEAR_API_KEY"}
}
func (a *app) listWorkspaces() error {
	list, err := discoverWorkspaces(configRoot(a.home, a.getenv))
	if err != nil {
		return err
	}
	return a.printJSON(map[string]any{"workspaces": list})
}
