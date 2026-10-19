# Verification

## Initial local verification

Verified 2026-10-19 using Go 1.23.12 on macOS/arm64:

- Offline unit/httptest integration suite: **74 tests/subtests passed**.
- Race detector and `go vet`: passed.
- Statement coverage at the initial implementation: **88.1%**.
- Embedded `--skills`, per-command help, version, and offline workspace discovery: passed.
- Cross-compilation: macOS, Linux, and Windows, each on amd64 and arm64: passed. Cross-compilation alone is not runtime verification of each target.
- No external Go dependencies or persistent runtime service.

## Authorized live verification

API source: https://api.linear.app/graphql
Retrieved at: 2026-10-19T03:17:16.896325+00:00

Two existing credential files were checked using read-only identity/team queries.
Live writes were restricted to two clearly marked test-created issues in a
user-approved team. Existing issues were not modified.

Verified through the compiled CLI:

- Identity, teams, users, states, labels, projects, cycles, filtered issue lists, search, details, and paginated comments.
- Create and re-read two issues with multiline descriptions.
- Update title, priority, team-scoped state, and assignee; clear assignee.
- Set and clear labels on test issues.
- Create, edit, read back, and delete the test's own comment.
- Verify blocks/blockedBy direction and remove the test relations.
- Set a real sub-issue parentId and detach it.
- Raw GraphQL variables.
- Archive/unarchive, followed by final archival cleanup.

Final checks confirmed both fixtures archived, with no test comments,
relationships, or parent links remaining. Full-text indexing can lag newly
created issues; exact identifier reads were used for authoritative checks.
Duplicate relations and project/cycle assignment were covered by offline mocks,
not live changes to real project/cycle membership.

Private reports remain in ignored `.live-test/`; internal workspace names,
issue links, and UUIDs are intentionally absent from this public summary.
Credentials were read in place and were not embedded in source or binaries.

## Automated release verification

The CI workflow runs native offline tests/build smoke checks on macOS, Linux,
and Windows, plus the race detector on Linux. The Release workflow invokes
those checks against the exact tagged commit before producing six archives and
SHA-256 checksums. Inspect the repository's Actions runs for current results.
