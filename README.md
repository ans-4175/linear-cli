# linear-cli

**Linear, minus the tab archaeology.**

Read an issue, change its status, leave a useful comment, get back to work.
A single, stateless Go binary talks directly to Linear's GraphQL API using your
existing token file. It handles the GraphQL paperwork without a browser login.

## Install

Download your build from [GitHub Releases](https://github.com/ans-4175/linear-cli/releases/latest).

| Platform | Intel / AMD 64-bit | ARM64 |
|---|---|---|
| macOS | `darwin_amd64.tar.gz` | `darwin_arm64.tar.gz` (Apple Silicon) |
| Linux | `linux_amd64.tar.gz` | `linux_arm64.tar.gz` |
| Windows | `windows_amd64.zip` | `windows_arm64.zip` |

Asset names start with `linear-cli_<version>_`. Each archive contains the binary,
this README, and an agent guide. `SHA256SUMS.txt` lists archive checksums; compare
your download's SHA-256 with its entry before running it. Binaries are unsigned,
so platform security prompts may apply.

For example, on an Apple Silicon Mac:

```sh
tar -xzf linear-cli_0.1.0_darwin_arm64.tar.gz
mkdir -p ~/.local/bin
install -m 755 linear-cli ~/.local/bin/linear-cli
linear-cli --version
```

Make sure `~/.local/bin` is on your PATH. On Windows, extract the ZIP and run
`linear-cli.exe`, or add its directory to PATH. No Go installation is needed
for a downloaded binary.

## Point it at your token file

Put your Linear Personal API Key in a plain-text file containing just the key.
Already have one? Leave it where it is.

```sh
linear-cli doctor --token-file /path/to/your/linear-token
```

The CLI reads that file when a command needs it. It doesn't copy the key into
a second credential store, rewrite your file, or make you negotiate with a
login screen. On macOS/Linux, keep the file readable only by you (`chmod 600`).

For fewer keystrokes, give the command a shell alias. The alias stores a path,
not the token:

```sh
alias lin='linear-cli --token-file /path/to/your/linear-token'
lin doctor
lin teams
```

Use `linear-cli.exe --token-file C:\path\to\linear-token doctor` on Windows.
The examples below use `lin`; the full command works just as well.

### More than one workspace?

Pick the token file for the command. No account migration ceremony required.

```sh
linear-cli teams --token-file /path/to/work-token
linear-cli teams --token-file /path/to/personal-token
```

Optional shortcuts: files at `~/.config/<alias>/linear-token` (or
`$XDG_CONFIG_HOME/<alias>/linear-token`) are discovered by `linear-cli workspaces`
and selected with `--workspace <alias>`. `--account` is an alias for that flag.
Verify `doctor` before writes: the token, not the local nickname, determines
the organization you're talking to.

Credential precedence:

1. `--api-key TOKEN` (available, but shell history is a terrible keychain)
2. `--token-file PATH`
3. `--workspace ALIAS`
4. `LINEAR_API_KEY`
5. `LINEAR_WORKSPACE`
6. A single auto-discovered token file

If discovery finds multiple files, the CLI asks you to choose rather than
playing workspace roulette. An explicit bad path/alias fails; it never silently
switches accounts. Personal API Keys use the raw Authorization header. For an
OAuth access token, put `Bearer <access-token>` in the file instead.

## Read first. Then do the thing.

```sh
lin issues --team ENG --state 'In Progress'
lin issue ENG-123 --with-comments
lin search 'login bug' --team ENG
lin comments ENG-123 --all

lin create --team ENG --title 'Fix login' \
  --desc-file /tmp/description.md --dry-run
# Inspect the plan. Remove --dry-run when you're ready to send it.

lin update ENG-123 --priority 2 --state 'In Progress'
lin comment ENG-123 --body-file /tmp/comment.md
lin relate ENG-123 ENG-456 --type blocks
lin archive ENG-123 --yes
```

## Command map

| Command | What it does |
|---|---|
| `doctor` / `whoami` | Check identity, organization, credential source |
| `workspaces` / `accounts list` | Discover local token-file aliases, offline |
| `issue` | Read detail, parent, labels, relation UUIDs, optional comments |
| `issues` / `search` | Filter or full-text search issues |
| `comments` | Paginate comments with their full UUIDs |
| `teams` / `users` / `states` | Resolve IDs; states need a team |
| `labels` / `projects` / `cycles` | Look up metadata; cycles need a team |
| `create` / `update` | Create an issue or change selected fields |
| `comment` / `update-comment` / `delete-comment` | Manage comments |
| `relate` / `unrelate` | blocks, blockedBy, related, duplicate, parent, child |
| `archive` / `unarchive` | Archive or restore, without permanent deletion |
| `graphql` | Run a custom query or mutation with variables |
| `--skills` / `--skill` / `skill` | Print the embedded agent guide |
| `--version` / `version` | Print the build version |

Run `linear-cli COMMAND --help` for flags. Global flags work before or after the
command; both `--flag value` and `--flag=value` are accepted.

### Instructions your agent can actually find

```sh
linear-cli --skills
```

The guide is embedded in the binary and works offline. It covers authentication,
ID lookups, pagination, write safety, relation direction, and failure handling.
No separate skill download required.

## Details worth knowing

- API results are JSON: readable by default, compact with `--json`.
- Use `--desc-file PATH` / `--body-file PATH` for multiline Markdown; `-` reads
  stdin. Quotes, Unicode, backslashes, and trailing newlines survive the request.
  Linear may normalize Markdown when storing it.
- Issue references accept a UUID, `ENG-123`, `eng123`, or a Linear issue URL.
  A bare number requires `--team KEY`. No guessing the first team.
- Team keys, state names, and assignee emails are resolved before mutation.
  Ambiguous matches fail instead of choosing someone with a similar name.
- Only supplied fields change. Use `none` to clear nullable assignee, parent,
  project, cycle, or due-date fields. `--desc ''` clears a description;
  `--label-ids ''` clears labels. A nonempty label list **replaces** the set.
- Lists return `nodes` and `pageInfo`. Use `--first 1..100`, `--after CURSOR`, or
  `--all`; `--first` is page size, not an overall limit with `--all`.
- `issue --with-comments` includes the first 25 comments; `comments --all` gets
  the rest. Nested labels/relations also have caps and expose `pageInfo`.
  Use raw GraphQL to paginate those connections when needed.

## Guardrails, not a permission slip

- `--dry-run` prints the mutation without sending it. Lookup queries may run.
  Dry-run isn't a live schema or permission check.
- `delete-comment`, `unrelate`, `archive`, and `unarchive` require `--yes` unless
  dry-run. Create/update/comment commands execute when invoked.
- There's no permanent issue-delete command.
- Raw `graphql` permits queries **and mutations**. You're holding the steering
  wheel; the CLI doesn't infer `success:false` inside arbitrary raw results.
- Requests aren't automatically retried. A timed-out create may have committed;
  check before repeating it. One bug report is usually enough.
- Redirects aren't followed. Production calls go to `https://api.linear.app/graphql`.
- HTTP 200 GraphQL errors and named mutations returning `success:false` fail.
  Errors go to stderr; `--json` includes structured, credential-redacted details.
- Exit codes: **0** success, **1** API/runtime failure, **2** usage/validation,
  **3** local credential configuration error.

## Build and test

Requires Go 1.23+ to build; uses only the standard library.

```sh
make build
make test           # offline unit/integration tests, race detector, go vet
make cross-check    # six OS/architecture build checks
make install        # ~/.local/bin/linear-cli
```

CI runs native tests on macOS, Linux, and Windows. Releases package amd64 and
arm64 for all three platforms, with SHA-256 checksums. ARM64 cross-builds aren't
a claim that every target has been runtime-tested.

Live tests are opt-in, create two clearly marked fixtures, and archive them at
the end. They can send normal Linear notifications. Don't run them in someone
else's workspace without permission:

```sh
python3 scripts/live_smoke.py --workspace your-alias --team ENG \
  --expected-org your-org-slug --allow-write
```

Private reports stay in ignored `.live-test/`. A non-sensitive verification
summary lives in [docs/verification.md](docs/verification.md).

### Publish a release

Push a version tag after committing the intended source:

```sh
git tag -a v0.2.0 -m 'linear-cli v0.2.0'
git push origin main v0.2.0
```

The Release workflow tests the tagged commit on all three operating systems,
builds six archives, checks the asset set, and uploads to a draft before
publishing. No Linear credentials are needed in CI. Manual dispatch accepts an
existing version tag. Published releases aren't silently overwritten on rerun.
