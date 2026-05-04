# FAQ

## Is Conduit free?

Yes. MIT-licensed. You bring your own API keys (or run local models).

## Does Conduit phone home?

No. There's no telemetry. The local `usage.db` ledger never leaves disk.

## Which platforms are supported?

macOS is the primary target (Apple Silicon and Intel). The core engine is
pure Go and runs on Linux for headless deployments; the GUI is macOS-only.

## Can I use it with local models?

Yes. Anything OpenAI- or Anthropic-compatible — Ollama, vLLM, LM Studio,
LiteLLM — drops in as a provider.

## Where do I file bugs and feature requests?

[GitHub issues](https://github.com/jabreeflor/conduit/issues).

## How do I contribute?

See [`CONTRIBUTING.md`](https://github.com/jabreeflor/conduit/blob/main/CONTRIBUTING.md)
in the main repo.
