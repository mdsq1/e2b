# AGENTS.md

## Cursor Cloud specific instructions

This is the **E2B Go SDK** — a zero-dependency Go client library for the E2B cloud sandbox platform. There are no local services to run; the SDK communicates with the remote E2B cloud API.

### Key commands

| Task | Command |
|---|---|
| Build | `go build ./...` |
| Vet / lint | `go vet ./...` |
| Unit tests (no API key) | `go test -v -count=1 ./...` |
| Integration tests | `E2B_API_KEY=sk-... go test -v -run 'TestIntegration' ./...` |

### Notes

- **Zero external Go dependencies** — only the Go standard library is used; `go mod download` is a no-op.
- **Integration tests** require a valid `E2B_API_KEY` environment variable and make real API calls to `api.e2b.app`. They are automatically skipped when the key is not set.
- **Unit tests** cover local logic (parsing, options, errors, serialization, template builder) and require no network access or API key. All unit tests run in-process with mock HTTP servers.
- Example programs live in `examples/` — they compile but require `E2B_API_KEY` to actually run against the cloud.
- The Go module path is `github.com/mdsq1/e2b` (see `go.mod`).
- `TemplateBuilder.ToJSON()` returns `(string, error)`, not `([]byte, error)`.
- This is a **library**, not a runnable service. There are no servers, Docker containers, or databases to start. The "hello world" for this SDK is building and running a Go program that imports the package and exercises its local APIs (template builder, client config, error types).
