# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: CLI entrypoint; injects build metadata into `internal/version`.
- `cmd/`: Cobra commands (`root`, `version`, `plugin`, `init`, `update`).
- `internal/`: non-exported packages:
  - `plugin/` for plugin discovery/install/exec + manifest handling.
  - `selfupdate/` for CLI self-update flow.
  - `githubrelease/` shared GitHub release fetch/extract helpers.
  - `version/` build/version string formatting.
- `scripts/install.sh`: installer used for release-based bootstrap.
- `.github/workflows/`: CI (`go vet`, `go test`, `go build`) and tagged release via GoReleaser.

## Build, Test, and Development Commands
- `make build`: builds `./clime` with ldflags metadata.
- `make install`: installs via `go install` and runs `clime init`.
- `make test`: runs `go test ./... -v`.
- `make lint`: runs `go vet ./...`.
- `make clean`: removes local `clime` binary.
- Quick local run: `go run . version` or `go run . update --help`.

## Coding Style & Naming Conventions
- Language: Go (`go 1.26.x` per `go.mod`).
- Formatting: always run `gofmt -w` on changed Go files.
- Keep packages small and single-purpose under `internal/`.
- File and package names should be lowercase; command files in `cmd/` should map to subcommands (for example, `update.go` -> `clime update`).
- Error messages should wrap context (`fmt.Errorf("...: %w", err)`).
- Design carefully when creating new commands, including command API and member function signatures (name, parameters, and return values).
- Keep functionality between commands orthogonal and clean.
- Avoid ad hoc helper functions; prioritize maintainable, well-structured code.
- Reuse existing structs (e.g., `plugin.Plugin`, `plugin.UpdateOptions`) for command flag bindings instead of declaring separate variables. Bind flags directly to struct fields.

## Testing Guidelines
- Framework: Go `testing` package.
- Place tests as `*_test.go` next to implementation files.
- Prefer table-driven tests for parsing/matching logic.
- Run full suite before PR: `go test ./... -v` (CI runs this plus `go vet` and a build).

## Commit & Pull Request Guidelines
- This branch currently has no commit history; use Conventional Commit style for consistency:
  - `feat: add self-update rollback check`
  - `fix(plugin): handle missing release asset`
  - `docs: update install notes`
- Keep commits focused and logically scoped.
- PRs should include:
  - purpose and behavior change summary,
  - test evidence (command + result),
  - linked issue/ticket when applicable,
  - CLI output snippets for user-facing changes.

## Security & Configuration Tips
- Use `GITHUB_TOKEN` for private repos or higher GitHub API limits.
- Do not hardcode secrets; pass via environment variables.
- Validate downloaded release assets by exact binary name and OS/arch match.
