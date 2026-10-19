# Linear from your terminal

Download the archive for your OS and CPU, extract it, and run `linear-cli --skills`.

- macOS: `darwin_amd64` (Intel) or `darwin_arm64` (Apple Silicon).
- Linux: `linux_amd64` or `linux_arm64`.
- Windows: `windows_amd64` or `windows_arm64` (ZIP with `linear-cli.exe`).

Every archive includes the binary, README, and agent guide. Verify the archive against `SHA256SUMS.txt` before running it. Binaries are unsigned; platform security prompts may apply. No Go runtime is required.

## Credentials stay in their file

```sh
linear-cli doctor --token-file /path/to/your/linear-token
linear-cli --skills
```

Point to an existing file containing your Personal API Key (or `Bearer <token>` for OAuth). The CLI reads it in place and does not create a second credential store.

Supports issue read/search/create/update, comments, relationships, sub-issues, metadata lookups, archive/unarchive, raw GraphQL, pagination, JSON output, and dry-run. Tests run on macOS, Linux, and Windows before release; all six targets are cross-compiled. Live workspace tests are never run by CI.
