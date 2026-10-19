package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

type options struct {
	Command string
	Args    []string
	Flags   map[string]string
}

var boolFlags = map[string]bool{"help": true, "json": true, "dry-run": true, "all": true, "with-comments": true, "include-archived": true, "yes": true}
var valueFlags = strings.Fields("workspace token-file api-key timeout first after team team-id state state-id assignee assignee-id title desc desc-file priority label-ids project-id cycle-id parent-id due-date name email body body-file type query-file variables variables-file")
var flagAliases = map[string]string{"h": "help", "account": "workspace", "token": "api-key", "description": "desc", "description-file": "desc-file", "limit": "first"}

func parseArgs(args []string) (options, error) {
	o := options{Flags: map[string]string{}}
	known := map[string]bool{}
	for _, f := range valueFlags {
		known[f] = true
	}
	literal := false
	for i := 0; i < len(args); i++ {
		s := args[i]
		if !literal && s == "--" {
			literal = true
			continue
		}
		if !literal && (s == "--skill" || s == "--skills") {
			if o.Command != "" {
				return o, usageError("--skills must be used as the command")
			}
			o.Command = "skill"
			continue
		}
		if !literal && (s == "--version" || s == "-v") {
			if o.Command != "" {
				return o, usageError("--version must be used as the command")
			}
			o.Command = "version"
			continue
		}
		if !literal && strings.HasPrefix(s, "-") && s != "-" {
			key, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(s, "-"), "-"), "=")
			if alias, ok := flagAliases[key]; ok {
				key = alias
			}
			if boolFlags[key] {
				if hasValue {
					b, e := strconv.ParseBool(value)
					if e != nil {
						return o, usageError("--%s requires true or false", key)
					}
					value = strconv.FormatBool(b)
				} else {
					value = "true"
				}
			} else {
				if !known[key] {
					return o, usageError("unknown flag --%s", key)
				}
				if !hasValue {
					i++
					if i >= len(args) || strings.HasPrefix(args[i], "--") {
						return o, usageError("--%s requires a value", key)
					}
					value = args[i]
				}
			}
			if _, exists := o.Flags[key]; exists {
				return o, usageError("flag --%s supplied more than once", key)
			}
			o.Flags[key] = value
			continue
		}
		if o.Command == "" {
			o.Command = s
		} else {
			o.Args = append(o.Args, s)
		}
	}
	if o.Command == "--help" {
		o.Command = "help"
	}
	return o, nil
}
func (o options) get(k string) string    { return o.Flags[k] }
func (o options) has(k string) bool      { return o.Flags[k] == "true" }
func (o options) supplied(k string) bool { _, ok := o.Flags[k]; return ok }
func (o options) requireArgs(n int) error {
	if len(o.Args) != n {
		return usageError("%s requires %d positional argument(s); see linear-cli %s --help", o.Command, n, o.Command)
	}
	return nil
}
func (o options) validate(extra string) error {
	allowed := map[string]bool{}
	for _, k := range strings.Fields("help json workspace token-file api-key timeout " + extra) {
		allowed[k] = true
	}
	for k := range o.Flags {
		if !allowed[k] {
			return usageError("--%s is not supported by %s", k, o.Command)
		}
	}
	return nil
}
func (o options) first() (int, error) {
	n := 25
	if o.supplied("first") {
		var e error
		n, e = strconv.Atoi(o.get("first"))
		if e != nil || n < 1 || n > 100 {
			return 0, usageError("--first must be between 1 and 100")
		}
	}
	return n, nil
}
func (o options) confirmed() error {
	if !o.has("yes") && !o.has("dry-run") {
		return usageError("%s changes/removes data; supply --yes (or inspect --dry-run first)", o.Command)
	}
	return nil
}

var commandHelp = map[string]string{
	"issue":          "issue <KEY-123|UUID|URL> [--team KEY] [--with-comments]\nRead issue, labels, parent, relationships, and optional first page of comments.",
	"comments":       "comments <issue> [--team KEY] [--first N] [--after CURSOR] [--all]\nRead paginated issue comments, including full UUIDs.",
	"issues":         "issues [--team KEY|--team-id UUID] [--state NAME|--state-id UUID] [--assignee EMAIL|--assignee-id UUID] [--project-id UUID] [--cycle-id UUID]\nList issues with server-side filters.",
	"search":         "search <text> [--team KEY|--team-id UUID]\nSearch issue text using Linear's searchIssues endpoint.",
	"teams":          "teams\nList teams and UUIDs.",
	"users":          "users [--name TEXT] [--email TEXT]\nList or find users.",
	"states":         "states --team KEY|--team-id UUID\nList workflow states for a team.",
	"labels":         "labels [--team KEY|--team-id UUID]\nList labels; a team filter includes workspace-wide labels.",
	"projects":       "projects [--team KEY|--team-id UUID]\nList projects (team filter uses accessibleTeams).",
	"cycles":         "cycles --team KEY|--team-id UUID\nList cycles for a team.",
	"create":         "create --title TEXT --team KEY|--team-id UUID [issue fields] [--dry-run]\nCreate issue. Fields: --desc TEXT|--desc-file PATH, --priority 0..4,\n--state NAME|--state-id UUID, --assignee EMAIL|--assignee-id UUID,\n--label-ids UUID,UUID, --project-id UUID, --cycle-id UUID, --parent-id ISSUE, --due-date YYYY-MM-DD.",
	"update":         "update <issue> [--title TEXT] [issue fields] [--dry-run]\nUpdate only supplied fields. Same fields as create except team is a lookup hint, not a move.\nClear nullable fields with none: --assignee-id none, --parent-id none,\n--project-id none, --cycle-id none, --due-date none. --label-ids '' clears labels.\nUse --desc '' or an empty description file to clear the description.",
	"comment":        "comment <issue> --body TEXT|--body-file PATH [--dry-run]\nCreate a comment. File '-' reads stdin and preserves multiline content.",
	"update-comment": "update-comment <full-comment-UUID> --body TEXT|--body-file PATH [--dry-run]\nReplace the body of an existing comment.",
	"delete-comment": "delete-comment <full-comment-UUID> --yes [--dry-run]\nDelete a comment. Requires its full UUID.",
	"relate":         "relate <issue-A> <issue-B> [--type related|blocks|blockedBy|duplicate|parent|child] [--dry-run]\nA blocks B; A blockedBy B becomes B blocks A. A parent B sets B's parent to A.\nA child B sets A's parent to B. Parent/child are issue updates, not relations.",
	"unrelate":       "unrelate <relationship-UUID> --yes [--dry-run]\nDelete a relationship. To detach a child, use update CHILD --parent-id none.",
	"archive":        "archive <issue> --yes [--dry-run]\nArchive an issue without permanently deleting it.",
	"unarchive":      "unarchive <issue> --yes [--dry-run]\nRestore an archived issue.",
	"graphql":        "graphql '<query-or-mutation>' [--variables JSON|--variables-file PATH] [--dry-run]\ngraphql --query-file PATH|- [--variables JSON|--variables-file PATH]\nWith no query argument/file, reads query from stdin. Escape hatch: raw mutations are allowed.",
	"doctor":         "doctor\nCheck token, authenticated user, organization, and auth source. Never print the token.",
	"whoami":         "whoami\nAlias of doctor.",
	"workspaces":     "workspaces [list]\nDiscover ~/.config/*/linear-token (or $XDG_CONFIG_HOME). Offline; tokens are not printed.",
	"accounts":       "accounts list\nAlias of workspaces list; does not copy credentials.",
	"skill":          "skill | --skill | --skills\nPrint the embedded agent guide. No authentication or network required.",
	"version":        "version | --version\nPrint build version.",
}

func writeHelp(w io.Writer, command string) error {
	if command != "" {
		s, ok := commandHelp[command]
		if !ok {
			return usageError("no help for command %q", command)
		}
		fmt.Fprintln(w, "linear-cli "+s)
	} else {
		fmt.Fprintln(w, "linear-cli "+version+" — Linear GraphQL CLI for humans and agents")
		fmt.Fprint(w, `
Usage: linear-cli [global flags] <command> [arguments] [flags]

Read:
  doctor / whoami           Identity, organization, credential source
  workspaces / accounts    List token-file aliases (offline)
  issue ISSUE              Read issue details and relationship UUIDs
  issues / search TEXT     Filter or search issues
  comments ISSUE           Read paginated comments
  teams / users / states   Resolve identifiers
  labels / projects / cycles

Write:
  create / update          Create or update issue fields
  comment / update-comment / delete-comment
  relate / unrelate        Relations and parent/child issues
  archive / unarchive      Reversible issue lifecycle, --yes required
  graphql                  Raw GraphQL with variables

Agent guide: --skills     Version: --version     Command help: COMMAND --help
`)
	}
	fmt.Fprint(w, `
Global flags (before or after command):
  --workspace ALIAS       ~/.config/ALIAS/linear-token; alias --account
  --token-file PATH       Read existing credential file
  --api-key TOKEN         Explicit override (prefer files/env to avoid shell history)
  --timeout 30s           HTTP timeout (>0, <=10m)
  --json                  Compact JSON; errors are JSON on stderr
List flags: --first 1..100 --after CURSOR --all --include-archived
Write flags: --dry-run (no mutation; lookup queries may run)
Content flags: --desc-file PATH|- / --body-file PATH|- for multiline/stdin
Never automatically retries requests. Raw GraphQL permits mutations.
`)
	return nil
}
