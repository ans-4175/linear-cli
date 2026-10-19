# linear-cli maintainer notes

- Go 1.23+, standard library only; flat package main like figma-cli. Do not add a daemon or a duplicate secret store.
- main.go: dispatch/output/exit codes. args.go: global/interspersed flag parser and per-command help.
- auth.go: read existing per-alias token files. Explicit selection must not fall back. Never print/commit credentials.
- client.go: fixed production endpoint, JSON variables, timeout, redirect refusal, GraphQL errors even HTTP 200, no retries.
- data.go: issue normalization, ID resolution, cursor pagination. Reject ambiguous matches and stalled cursors.
- commands_read.go: issue details and lists. commands_write.go: issue/comment/relation lifecycle and dry-run. graphql.go: raw escape hatch.
- skill.go embeds skills/linear-cli.md. Update help, skill, README, and tests together whenever command semantics change.
- stdout is JSON for API commands; stderr is errors only. Help/skills/version remain text.
- Preserve exact file/stdin content in variables; never interpolate user content into GraphQL source.
- Update only supplied fields. Explicit null/empty fields are meaningful. Labels replace, not append.
- Every named mutation must return success:true. Never auto-retry mutations; failures can be ambiguous.
- Dry-run must send no mutation, though lookups may execute. Raw GraphQL intentionally allows arbitrary mutations.
- No permanent issue deletion. Archive/unarchive/delete-comment/unrelate require --yes unless dry-run.
- Run gofmt, go test -race -cover, go vet, and cross-build checks. Tests must not read actual credentials.
- Live write tests need explicit approval of workspace/team. Use only the two test-created issues, reconcile failed creates by unique title, archive fixtures, and verify cleanup.
- Do not edit linear.sh, figma-cli, penpot-cli, global agent skills, or credentials as part of maintenance without a separate request.
