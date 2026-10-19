package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"
)

var version = "dev"

type cliError struct {
	Code    int
	Message string
}

func (e *cliError) Error() string                 { return e.Message }
func usageError(format string, args ...any) error { return &cliError{2, fmt.Sprintf(format, args...)} }

type app struct {
	in          io.Reader
	out, errOut io.Writer
	home        string
	getenv      func(string) string
	endpoint    string
	client      *apiClient
	opts        options
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "linear-cli: cannot determine home directory")
		os.Exit(1)
	}
	a := &app{in: os.Stdin, out: os.Stdout, errOut: os.Stderr, home: home, getenv: os.Getenv, endpoint: apiEndpoint}
	os.Exit(a.run(ctx, os.Args[1:]))
}

func (a *app) run(ctx context.Context, args []string) int {
	opts, err := parseArgs(args)
	a.opts = opts
	if err == nil {
		err = a.execute(ctx)
	}
	if err == nil {
		return 0
	}
	code := 1
	var ce *cliError
	if errors.As(err, &ce) {
		code = ce.Code
	}
	if a.opts.has("json") {
		result := map[string]any{"error": err.Error(), "exitCode": code}
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			result["details"] = apiErr
		}
		_ = json.NewEncoder(a.errOut).Encode(result)
	} else {
		fmt.Fprintf(a.errOut, "linear-cli: %v\n", err)
	}
	return code
}

func (a *app) api() (*apiClient, error) {
	if a.client != nil {
		return a.client, nil
	}
	auth, err := resolveAuth(a.opts, a.home, a.getenv)
	if err != nil {
		return nil, err
	}
	timeout := 30 * time.Second
	if s := a.opts.get("timeout"); s != "" {
		timeout, err = time.ParseDuration(s)
		if err != nil || timeout <= 0 || timeout > 10*time.Minute {
			return nil, usageError("--timeout must be a duration >0 and <=10m (e.g. 30s)")
		}
	}
	a.client = newAPIClient(a.endpoint, auth.Token, timeout)
	return a.client, nil
}

func (a *app) query(ctx context.Context, query string, vars map[string]any) (map[string]any, error) {
	c, err := a.api()
	if err != nil {
		return nil, err
	}
	return c.do(ctx, query, vars)
}

func (a *app) printJSON(value any) error {
	enc := json.NewEncoder(a.out)
	enc.SetEscapeHTML(false)
	if !a.opts.has("json") {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(value)
}

func (a *app) execute(ctx context.Context) error {
	o := a.opts
	if o.has("help") || o.Command == "" || o.Command == "help" {
		target := o.Command
		if target == "help" || target == "" {
			target = ""
			if len(o.Args) > 0 {
				target = o.Args[0]
			}
		}
		return writeHelp(a.out, target)
	}
	switch o.Command {
	case "version":
		fmt.Fprintln(a.out, "linear-cli "+version)
		return nil
	case "skill":
		fmt.Fprint(a.out, skillMD)
		return nil
	case "workspaces", "accounts":
		if len(o.Args) > 1 || len(o.Args) == 1 && o.Args[0] != "list" {
			return usageError("use linear-cli workspaces [list]")
		}
		if err := o.validate(""); err != nil {
			return err
		}
		return a.listWorkspaces()
	case "doctor", "whoami":
		if err := o.requireArgs(0); err != nil {
			return err
		}
		if err := o.validate(""); err != nil {
			return err
		}
		data, err := a.query(ctx, "query Identity { viewer { id name email } organization { id name urlKey } }", nil)
		if err != nil {
			return err
		}
		auth, err := resolveAuth(o, a.home, a.getenv)
		if err != nil {
			return err
		}
		data["authSource"] = auth.Source
		return a.printJSON(data)
	case "issue":
		return a.readIssue(ctx)
	case "comments":
		return a.readComments(ctx)
	case "issues", "search", "teams", "users", "states", "labels", "projects", "cycles":
		return a.readList(ctx)
	case "create", "update":
		return a.writeIssue(ctx)
	case "comment", "update-comment", "delete-comment":
		return a.writeComment(ctx)
	case "relate", "unrelate":
		return a.writeRelation(ctx)
	case "archive", "unarchive":
		return a.archiveIssue(ctx)
	case "graphql":
		return a.rawGraphQL(ctx)
	default:
		return usageError("unknown command %q; run linear-cli --help", o.Command)
	}
}
