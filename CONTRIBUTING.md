# Contributing to vmbench

Thanks for helping improve vmbench. Bug reports, feature ideas, documentation fixes, benchmark adapters, platform compatibility fixes, and focused pull requests are welcome.

中文或英文 issue / PR 都欢迎。

## Project principles

- Keep reports based on raw measured metrics:
  - median time
  - throughput
  - latency
  - detail / error
- Do not reintroduce benchmark total scores, grades, category scores, or similar synthetic ranking fields.
- IP Quality risk scores are business diagnostics and are not benchmark scores.
- Prefer small, focused changes.
- Avoid new dependencies unless there is a clear reason.
- Do not revert unrelated user changes in the working tree.

## Project layout

- `bench/` — workload interface and native/network probes
- `catalog/` — external-tool workload registry and parsers
- `cmd/vmbench/` — CLI and TUI entrypoint
- `report/` — console / JSON / HTML / compare output
- `suite/` — VPS composite suite sections
- `sysinfo/` — system information collection
- `tui/` — Bubble Tea / Lip Gloss terminal UI
- `docs/` — product, architecture, changelog, and TUI documents

## Compatibility expectations

- `run`, `sysinfo`, and core report generation should remain cross-platform.
- `suite` network sections may fail in restricted environments, but failures must be written as structured errors instead of crashing.
- Hardware benchmarks use external tools such as `sysbench`, `fio`, and `openssl`; missing tools should remain structured workload errors.
- Official source and release archives do not vendor third-party benchmark tools by default. Optional local Linux fallbacks may be placed in `binaries/`, but that directory is gitignored.
- CLI JSON output and report schemas should remain compatible unless a breaking change is explicitly intended.

## Local development

```bash
go test ./...
go vet ./...
go build -o /root/temp/vmbench ./cmd/vmbench

./vmbench list
./vmbench sysinfo --json
./vmbench run --filter 'SHA|AES' --iterations 1 --quiet --json /tmp/vmbench.json
```

Notes:

- TUI needs a real TTY; for non-interactive verification, prefer CLI smoke checks.
- Network probes may fail in sandboxes or restricted VPS environments. That is acceptable when the failure is structured in the report.
- If the default Go cache is not writable, use `GOCACHE=/tmp/vmbench-gocache`.

## Pull request checklist

- [ ] The change is focused and clearly described
- [ ] Modified Go files have been formatted with `gofmt -w`
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `go build -o /root/temp/vmbench ./cmd/vmbench` passes
- [ ] README / docs / tests were updated when CLI, JSON schema, TUI, or report output changed
- [ ] No benchmark total score, grade, or category score was introduced
- [ ] No unrelated files were reverted

## Release checklist

```bash
go mod tidy
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go build -o /root/temp/vmbench ./cmd/vmbench
git diff --check
```

For a tagged release, push a `v*` tag to trigger GoReleaser:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
