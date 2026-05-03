# Install

Conduit is distributed as a single Go binary. macOS (Apple Silicon and Intel)
is the supported platform.

## Prerequisites

- macOS 13+
- Go 1.24+ (only required to build from source)
- An API key for at least one provider (Anthropic, OpenAI, OpenRouter, …) or a
  local Ollama / vLLM endpoint.

## Build from source

```sh
git clone https://github.com/jabreeflor/conduit.git
cd conduit
make build
./dist/conduit --help
```

To install into a system path:

```sh
cp dist/conduit /usr/local/bin/conduit
```

## Verify

```sh
conduit --version
```

You're ready for the [first run](./first-run).
