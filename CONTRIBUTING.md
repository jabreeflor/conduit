# Contributing to Conduit

Thanks for taking the time to contribute. This document is the source of
truth for development setup, conventions, and the PR workflow. The
[Code of Conduct](./CODE_OF_CONDUCT.md) governs all participation.

## One-command dev setup

```sh
git clone https://github.com/jabreeflor/conduit.git
cd conduit
make build
```

`make build` produces `dist/conduit`. There's no Node, Python, or system
package step — Conduit is pure Go (1.24+).

Optional: install the `gh` CLI to use the PR helpers in `.scripts/`.

## Project layout

```
cmd/conduit/        # binary entry point
internal/           # all production code (everything is internal/)
  router/           # provider routing & failover
  sessions/         # JSONL session store
  memory/           # pluggable memory providers
  hooks/            # subprocess hook runtime
  workflow/         # YAML workflow engine
  coding/           # `conduit code` REPL
  tools/            # built-in tool implementations
  tui/              # Bubble Tea TUI
  gui/              # GUI view-models
  …
docs/               # PRD, ADRs, design system, sprint plans
website/            # VitePress documentation site
.github/workflows/  # CI
```

For a deeper architectural tour, see
[`website/docs/architecture/`](./website/docs/architecture/index.md) and
the [ADRs](./docs/adr/).

## Code style

- **`gofmt -s`** — every Go file. CI fails on a non-empty `gofmt -l`.
- **`go vet ./...`** — must be clean.
- **Tests** — `go test ./...` must pass. New packages start with a
  `_test.go` file alongside the implementation.
- **No new top-level packages** without a short justification in the PR
  body. Prefer extending an existing `internal/<domain>/` package.
- **Prefer the standard library.** Conduit's only runtime deps are
  `bubbletea`, `bubbles`, `lipgloss`, `yaml.v3`, and `modernc.org/sqlite`.
  New deps need explicit reviewer sign-off.
- **No `init()` side-effects** that touch the filesystem, network, or
  global state.

Run all checks at once:

```sh
make lint typecheck test
```

## Commit messages — Conventional Commits

We use [Conventional Commits] so the changelog can be generated
automatically. Format:

```
<type>(#<issue>): <imperative summary>

<optional body>
```

`<type>` is one of:

| Type       | Meaning                                                      | Changelog section |
| ---------- | ------------------------------------------------------------ | ----------------- |
| `feat`     | New user-visible functionality                               | **Added**         |
| `fix`      | Bug fix                                                      | **Fixed**         |
| `perf`     | Performance improvement                                      | **Changed**       |
| `refactor` | Internal restructure, no behavior change                     | **Changed**       |
| `docs`     | Docs only                                                    | **Changed**       |
| `test`     | Tests only                                                   | _hidden_          |
| `build`    | Build / release tooling                                      | _hidden_          |
| `ci`       | CI workflows                                                 | _hidden_          |
| `chore`    | Anything that doesn't fit                                    | _hidden_          |

Breaking changes: append `!` after the type (e.g. `feat!:`) **or** add a
`BREAKING CHANGE:` footer. These land in a **Removed / Changed** section
with a prominent callout.

### Examples

```
feat(#137): add VitePress documentation site
fix(#88): handle nil provider in router cascade
refactor(#62): split sessions store into reader/writer
feat!(#13): rename `~/.conduit/keys.json` to keybindings.json
```

## Pull request process

1. **Open or claim an issue** before writing significant code.
2. **Branch** from `main` as `feat/issue-<N>-slug`,
   `fix/issue-<N>-slug`, etc.
3. **Make focused commits.** One PR == one logical change. Use multiple
   commits if it helps review; `feat(...)` commits drive the changelog.
4. **Run `make lint typecheck test` locally** before opening the PR.
5. **Reference the issue** in the PR body with `Closes #<N>`.
6. **PR title follows the same Conventional Commits format** as commit
   messages — the changelog generator reads PR titles preferentially when
   PRs are squashed.
7. **CI must be green** before merge. The merge button is gated on this.
8. **Squash merge** is the default. The squash commit message should be
   the PR title (already in the right format).

## Issue triage

We use a small label set so the project board stays legible:

| Label group | Labels                                                       |
| ----------- | ------------------------------------------------------------ |
| Type        | `bug`, `feature`, `docs`, `infra`, `ide`, `analytics`        |
| Priority    | `P0`, `P1`, `P2`, `P3`                                       |
| Wave        | `Wave-1` … `Wave-N` (release planning)                       |
| Status      | `Needs Review`, `Needs Repro`, `Blocked`, `Good First Issue` |

Maintainers triage new issues weekly. Anyone can suggest a label change
in a comment; a maintainer applies it.

## Releases & changelog

The changelog is generated from merged PRs by
`.scripts/gen-changelog.sh` and lives at [`CHANGELOG.md`](./CHANGELOG.md)
in [Keep a Changelog] format. To preview the next release section:

```sh
.scripts/gen-changelog.sh --since v0.1.0 --preview
```

To cut a release:

```sh
.scripts/gen-changelog.sh --release v0.2.0
git commit -am "chore: release v0.2.0"
git tag v0.2.0
git push --follow-tags
```

The `release` GitHub workflow then builds binaries and attaches the
relevant changelog section to the GitHub release notes.

## In-app changelog surfacing

When the binary detects that the previous run was an older version
(tracked in `~/.conduit/state.json`), the TUI shows a single-screen
"What's new in vX.Y.Z" panel sourced from the embedded `CHANGELOG.md`.
Users can dismiss with `Esc` or paginate with the arrow keys. Embedding
is wired up in `internal/changelog`.

## Getting help

- **Questions**: open a [GitHub Discussion](https://github.com/jabreeflor/conduit/discussions).
- **Bugs**: open an issue with `bug` label and a minimal repro.
- **Security**: please email the maintainer privately before filing a
  public issue.

[Conventional Commits]: https://www.conventionalcommits.org
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
