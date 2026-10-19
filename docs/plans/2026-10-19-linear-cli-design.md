# Linear CLI design

Approved by user: Go CLI in `~/workspaces/linear-cli`, following figma-cli's portable stdlib-only command/embedded-skill pattern. Live write tests explicitly approved in a designated workspace/team, using two dedicated test issues that must be archived afterward. Existing issues must not be modified. Private workspace and fixture identifiers are excluded from the public repository.

## Architecture

One stateless Go binary calling `https://api.linear.app/graphql` directly. No daemon, OAuth browser, shell dependency, or duplicated token store. JSON variables carry all user input; never concatenate content into GraphQL source. Use request timeouts, deny HTTP redirects, inspect GraphQL errors even with HTTP 200, and never automatically retry requests (particularly mutations).

Flat package-main layout like figma-cli: command dispatch/help/parser, auth discovery, GraphQL client, read commands, write commands, embedded agent skill, tests. Only Go standard library, minimum Go 1.23. No modifications to figma-cli, penpot-cli, linear.sh, or their skills.

## Authentication

Explicit `--api-key` > `--token-file` > `--workspace` alias > `LINEAR_API_KEY` > `LINEAR_WORKSPACE` alias > a single auto-discovered token file. An explicit invalid selection must fail, not fall back to another workspace. Alias files: `$XDG_CONFIG_HOME/<alias>/linear-token` or `~/.config/<alias>/linear-token`. Reject path traversal. When multiple aliases exist without another selection, ask the caller to select one. Support Personal API Keys and explicitly Bearer-prefixed OAuth credentials. Never print credentials. Workspace listing and offline help/skills need no network.

## Commands

Parity with linear.sh: issue, issues, search, create, update, comment, update-comment, delete-comment, relate, unrelate, users, teams, states, graphql. Additional commands: doctor/whoami, workspaces/accounts list, comments, labels, projects, cycles, archive, unarchive. Global `--workspace`, `--token-file`, `--api-key`, `--json`, `--timeout`, `--dry-run`. Built-in help per command and embedded `skill`/`--skill`/`--skills`.

Resolve UUIDs and issue identifiers (case-insensitive, dashless forms accepted); bare issue numbers require an explicit team rather than choosing the first team. Resolve team keys, workflow-state names scoped to the issue's team, and assignee emails, failing on zero/ambiguous matches. Nullable update fields can be explicitly cleared; labels replace only when supplied. Long text accepts inline, file, or stdin input without interpolation or truncating trailing newlines.

Lists expose cursor pagination, `--first`, `--after`, `--all`, `--include-archived`. Issue details expose comment/relationship identifiers and a dedicated paginated comments command. Mutations support dry-run (read-only lookups allowed). Archive, unarchive, delete-comment, and unrelate require `--yes` unless dry-run. No permanent issue-delete command. Raw GraphQL is an explicit escape hatch with variables and file/stdin support.

## Output and errors

JSON output for API commands, compact with `--json` and readable/indented otherwise; stdout only contains command results, errors go to stderr. Exit 0 success, 1 API/runtime failure, 2 usage/validation, 3 missing/invalid local credential configuration. Return pageInfo on lists; never silently claim a truncated list is complete. Preserve GraphQL partial data in structured API error results without exposing the token.

## Verification

Unit and httptest integration tests for flag parsing, auth precedence/traversal, content fidelity, HTTP/GraphQL errors, redirects, cursor handling, ambiguous lookups, relation direction, dry-run, failure payloads, and mutation variables. `go test -race`, `go vet`, cross-compilation checks, offline skills/help.

Live read test both configured workspaces; live writes only to two uniquely named smoke-test issues in the approved team. Exercise create/update/comment/edit/delete comment, issue relations/sub-issues, and archive/unarchive. Cleanup removes only test-created comments/relations as needed and archives both test issues, then re-reads them to verify archive status. Record non-secret evidence, URLs, and retrieved_at in the evidence ledger/report. Never automatically replay a failed create on an uncertain transport result.
