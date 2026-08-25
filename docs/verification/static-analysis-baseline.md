# Static-analysis baseline

Date: 2026-08-24

Toolchain:

- Go 1.24.0
- golangci-lint 2.1.5, built with Go 1.24.0
- Configuration: `.golangci.yml`

The inherited configuration explicitly disabled `errcheck`, `govet`, and `staticcheck`. Before source changes, each analyzer was run independently with unlimited issue reporting:

```powershell
golangci-lint run --enable-only=errcheck --max-issues-per-linter=0 --max-same-issues=0
golangci-lint run --enable-only=govet --max-issues-per-linter=0 --max-same-issues=0
golangci-lint run --enable-only=staticcheck --max-issues-per-linter=0 --max-same-issues=0
```

Recorded baseline:

| Analyzer | Findings | Resolution |
| --- | ---: | --- |
| `errcheck` | 7 | Close results are now explicitly acknowledged while preserving the existing public APIs and behavior. |
| `govet` | 4 | Four line-local exclusions document OpenGL buffer-offset arguments that are intentionally represented as pointer values. |
| `staticcheck` | 7 | Embedded selectors, boolean logic, and parser branches were simplified without changing behavior. |

The OpenGL exclusions are confined to `internal/platform/gl.go`. `gl.DrawElements` and `gl.VertexAttribPointer` interpret the pointer parameter as a byte offset into the currently bound GPU buffer. These values are not Go pointers and must not be converted or dereferenced as Go memory.

Final gate:

```text
golangci-lint run --max-issues-per-linter=0 --max-same-issues=0
0 issues.
```
