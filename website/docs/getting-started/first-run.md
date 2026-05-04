# First run

```sh
conduit
```

The first launch creates `~/.conduit/` with the default config, key store, and
session directory. You'll be prompted to register at least one provider.

## Add a provider

```sh
conduit provider add anthropic
```

Paste your API key when prompted. It's stored in the macOS Keychain — never on
disk in plaintext.

## Send a prompt

In the TUI, type a message and press Enter. The router picks the cheapest
provider that can handle the request and streams the response back.

## Where things live

| Path                       | What it is                                  |
| -------------------------- | ------------------------------------------- |
| `~/.conduit/config.yaml`   | Main config (providers, routing, hooks)     |
| `~/.conduit/sessions/`     | JSONL session logs                          |
| `~/.conduit/memory/`       | Memory provider data                        |
| `~/.conduit/usage.db`      | Local usage / cost ledger                   |

Next: [Concepts](./concepts).
