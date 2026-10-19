---
name: linear-cli
description: "Read, search, create, update, comment on, and manage Linear issues through a token-based Go CLI. Supports teams, states, labels, projects, cycles, relationships, sub-issues, archive, raw GraphQL, and pagination."
---

# Linear CLI

Run `linear-cli --help`, then `linear-cli COMMAND --help` for exact syntax. Read this embedded guide with `linear-cli --skills`. The binary is stateless and needs no daemon/browser.

## Point to an existing credential file

```sh
linear-cli doctor --token-file /path/to/linear-token --json
linear-cli teams --token-file /path/to/linear-token --json
```

Read the key from its existing file; never copy it into the repo, echo it, or paste it into shell history. Keep the file mode 0600 on macOS/Linux. A Personal API Key is one raw key line; an OAuth access token must be stored as `Bearer <token>`.

For optional aliases, discover `$XDG_CONFIG_HOME/<alias>/linear-token` (default `~/.config/<alias>/linear-token`) using `linear-cli workspaces`, then select `--workspace work` (alias `--account`). Never assume the alias equals the actual organization: verify `doctor` before writes.

Auth precedence: `--api-key` > `--token-file` > `--workspace` > `LINEAR_API_KEY` > `LINEAR_WORKSPACE` > a single discovered token file. Multiple aliases without a selection fail. Explicit bad paths/aliases never fall back to a different account. The CLI does not persist a `--token-file` selection: pass it on subsequent commands or define a shell alias containing the path.

In examples below, add `--token-file /path/to/linear-token` (or the selected workspace flag) to each authenticated command.

## Read and resolve

```sh
linear-cli teams
linear-cli issue ENG-123 --with-comments
linear-cli issues --team ENG --state 'In Progress' --first 25
linear-cli search 'login bug' --team ENG
linear-cli comments ENG-123 --all
linear-cli users --email 'someone@example.com'
linear-cli states --team ENG
linear-cli labels --team ENG
linear-cli projects --team ENG
linear-cli cycles --team ENG
```

Accept full issue UUIDs, case-insensitive identifiers (`ENG-123`, `eng123`), or `https://linear.app/.../issue/ENG-123/...` URLs. Bare numbers require `--team KEY`; never guess the first team. Comment/relation/state/project/cycle/label IDs are full UUIDs. Team/state/email lookups fail on zero or ambiguous matches.

Lists return `nodes` and `pageInfo`. `--first N` is page size (1..100), not a total count with `--all`. Continue with `--after CURSOR` or use `--all`. Top-level lists accept `--include-archived`. Issue detail exposes pageInfo for nested labels/relations/comments; it may be truncated. `issue --with-comments` returns the first 25 comments; use `comments --all` or explicit raw GraphQL pagination for complete results.

## Create and update

```sh
linear-cli create --team ENG --title 'Fix login' --desc-file /tmp/description.md --dry-run
# Execute the same command without --dry-run only after user authorization.
linear-cli update ENG-123 --state 'In Progress' --priority 2
linear-cli update ENG-123 --assignee 'someone@example.com'
linear-cli update ENG-123 --desc-file /tmp/new-description.md
```

Use `--desc-file PATH` / `--body-file PATH` for long or multiline content. `-` reads stdin. Request variables preserve quotes, backticks, backslashes, Unicode, newlines, and trailing newlines; Linear may normalize Markdown when storing it. Inline content also works when correctly shell-quoted. Never supply both inline and file forms.

Only supplied fields change. Priorities: 0 none, 1 urgent, 2 high, 3 normal, 4 low. `--state NAME` resolves in the issue's actual team. `--team` on update is a lookup/validation hint, not a move.

Clear nullable fields with `none`: `--assignee-id none`, `--parent-id none`, `--project-id none`, `--cycle-id none`, `--due-date none`. Clear descriptions with `--desc ''` or an empty file. `--label-ids ''` clears labels; `--label-ids UUID,UUID` replaces the complete set, not appends. Due dates accept YYYY-MM-DD.

## Comments

```sh
linear-cli comment ENG-123 --body-file /tmp/comment.md
linear-cli update-comment COMMENT_UUID --body-file /tmp/revised.md
linear-cli delete-comment COMMENT_UUID --dry-run
linear-cli delete-comment COMMENT_UUID --yes
```

Read existing content before replacing it. Get full comment UUIDs from `comments` or `issue --with-comments`. Do not post comments or edit someone else's work without authorization.

## Relations and sub-issues

```sh
linear-cli relate ENG-123 ENG-456 --type blocks
linear-cli relate ENG-123 ENG-456 --type blockedBy
linear-cli relate ENG-123 ENG-456 --type parent
linear-cli update ENG-456 --parent-id none
linear-cli unrelate RELATION_UUID --yes
```

`A blocks B` and `A blockedBy B` have opposite directions. `related` is the default. `duplicate` is directed; verify the source and target. `A parent B` sets B's parentId to A; `A child B` sets A's parentId to B. These create real sub-issues, not relation records. Detach a parent through `update CHILD --parent-id none`.

## Archive and raw GraphQL

```sh
linear-cli archive ENG-123 --dry-run
linear-cli archive ENG-123 --yes
linear-cli unarchive ENG-123 --yes
linear-cli graphql 'query One($id: String!) { issue(id: $id) { id identifier title } }' \
  --variables '{"id":"ENG-123"}'
linear-cli graphql --query-file /tmp/query.graphql --variables-file /tmp/vars.json
```

`delete-comment`, `unrelate`, `archive`, and `unarchive` require `--yes` unless dry-run. Ordinary create/update/comment commands execute when invoked. No permanent issue-delete command exists. Dry-run prints the mutation without sending it; read-only lookups may still run. It is not live verification.

Raw GraphQL permits queries AND mutations; the caller owns authorization and validation. The raw command does not infer `success:false`. With no query argument/file it reads stdin. Query and variables cannot both consume stdin. Prefer named commands for normal work.

## Output and failure handling

- API commands write JSON to stdout; `--json` makes it compact and produces structured errors on stderr. Help/skills/version are text.
- Exit 0 success, 1 API/runtime failure, 2 usage/validation, 3 local credential configuration. GraphQL errors can arrive with HTTP 200. Named mutations must return `success:true`.
- Never blindly retry a timed-out mutation. Re-read/search first: the write may already have committed. The CLI itself never automatically retries.
- Respect rate limits/Retry-After. Narrow filters, cap page sizes, and avoid polling.
- Redirects are not followed. Production requests use the fixed `https://api.linear.app/graphql` endpoint.
- Never expose credentials or unrelated private descriptions/user emails in reports.
- Read before writing, confirm workspace/team, preserve unrelated fields, and verify changes afterward.
