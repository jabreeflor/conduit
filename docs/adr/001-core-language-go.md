# ADR-001: Core Language — Go

**Status:** Accepted  
**Date:** 2026-04-29  
**Issue:** [#1 — Decide core language stack](https://github.com/jabreeflor/conduit/issues/1)

---

## Context

Conduit's core engine must handle concurrent model requests, streaming responses, long-running workflow orchestration, TUI rendering, and IPC with a growing set of surface layers (TUI, GUI, Spotlight, iOS widgets, voice). The engine needs to be:

- **Responsive under load** — routing, hook dispatch, memory queries, and model streaming all happen in parallel.
- **Fast to cold-start** — macOS launch-on-demand and Spotlight overlay require sub-100 ms startup.
- **Easy to ship as a single binary** — cross-compilation and zero-dependency deployment are table stakes for a local-first tool.
- **Maintainable long-term** — the project starts as a solo build; the language must support confident refactoring without a large team.

Three candidates were evaluated: Python, Go, and Rust.

---

## Decision

**Go** is the core language for the Conduit engine.

---

## Evaluation

### Python

| Factor | Assessment |
|---|---|
| Iteration speed | Fastest — rich ecosystem, no compile step |
| AI/ML tooling | Best-in-class (Anthropic SDK, LangChain, llama-cpp-python, etc.) |
| Concurrency | Adequate with asyncio; GIL limits true parallelism |
| Cold start | Slow (200–500 ms+ for a non-trivial import graph) |
| Binary distribution | Poor — requires runtime or PyInstaller hacks |
| Memory safety | Dynamic typing; runtime errors common at boundaries |

Python's AI ecosystem advantage disappears for Conduit because the engine **talks to models over HTTP**, not via Python bindings. The Anthropic and OpenAI REST APIs are language-agnostic. Whisper.cpp, Piper, and LanceDB all expose C or gRPC interfaces that Go can call equally well. Cold-start and binary distribution are disqualifying for a macOS system tool.

### Rust

| Factor | Assessment |
|---|---|
| Performance ceiling | Highest — zero-cost abstractions, no GC pauses |
| Memory safety | Strongest — compile-time guarantees |
| Concurrency | Excellent (Tokio async runtime) |
| Cold start | Excellent |
| Binary distribution | Excellent |
| Dev velocity | Slowest — borrow checker friction, longer compile times |
| TUI ecosystem | Good (Ratatui), but smaller than Go's Bubbletea community |

Rust is the right choice when performance and safety cannot be traded off (e.g., sandboxing primitives, a high-throughput inference server). For Conduit's engine, the bottleneck is always the model API — not CPU cycles in the router. Rust's compile-time overhead and borrow-checker friction would slow Phase 1 iteration without a measurable user-facing benefit.

### Go

| Factor | Assessment |
|---|---|
| Concurrency | Excellent — goroutines + channels are a natural fit for streaming, hook dispatch, and parallel model calls |
| Cold start | ~10–30 ms for a typical binary |
| Binary distribution | Single static binary, cross-compile to arm64/amd64 with one command |
| TUI ecosystem | Bubbletea + Lipgloss — production-quality, actively maintained |
| Memory safety | Memory-safe; GC pauses are negligible at Conduit's scale |
| Dev velocity | Fast — simple language spec, fast compiler, excellent tooling (`gopls`, `go test`) |
| AI tooling | Thin but sufficient — Anthropic/OpenAI REST calls need only `net/http`; CGO bridges Whisper.cpp and LanceDB |

Go is the only candidate that is simultaneously fast to start, easy to ship, highly concurrent, and fast to write. Its TUI story (Bubbletea) is the strongest of the three for Phase 1.

---

## Consequences

### Positive

- Phase 1 delivers a single `conduit` binary installable via `brew` or a curl script.
- The goroutine model maps directly onto Conduit's architecture: one goroutine per streaming model call, one per hook subprocess, one per workflow step — with channels for backpressure.
- `go build -o conduit-arm64 GOARCH=arm64` gives a macOS Silicon binary from any machine.
- Bubbletea's Elm-style message loop integrates cleanly with Conduit's hook system.
- `go test` and `golangci-lint` keep the codebase healthy without a large CI investment.

### Negative / Mitigations

- **Weaker AI SDK ecosystem** — mitigated by the fact that all model I/O is REST/streaming JSON; the Go `net/http` client plus `bufio.Scanner` handles SSE streaming cleanly.
- **CGO required for some native libs** (Whisper.cpp, libimobiledevice) — acceptable; CGO is stable and these are optional, late-phase dependencies.
- **No generics-heavy abstractions before Go 1.18 patterns are well-understood** — target Go 1.22+ throughout; generics are used only where they reduce duplication, not speculatively.

---

## Interaction with TUI Stack Decision

The TUI stack issue (#TUI-stack) should default to **Bubbletea + Lipgloss** given this ADR. Both are pure-Go libraries with no CGO, no external processes, and first-class support for mouse, Unicode, and 256-color / true-color terminals. The Elm-style message loop makes the TUI a straightforward consumer of the engine's event bus.

---

## Alternatives Considered After Decision

- **Hybrid (Go engine + Python scripts)** — rejected. Two runtimes double the distribution complexity and create a marshalling layer that is a persistent source of bugs.
- **Go core + Rust hot paths** — rejected for Phase 1. Premature optimization; revisit if profiling reveals a real bottleneck in Phases 4–7.
