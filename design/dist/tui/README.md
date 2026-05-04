# conduit-design (Python)

Textual/Rich theme JSON for the Conduit design system. Generated from
[`design/tokens.yaml`](https://github.com/jabreeflor/conduit/blob/main/design/tokens.yaml)
by `cmd/design-tokens`.

## Install

```bash
pip install conduit-design
```

## Usage

```python
from conduit_design import load_theme

theme = load_theme("dark")  # or "light" / "hc"
```

The host TUI loads the same JSON files at startup, so plugin TUI panels
inherit identical colors and typography by reading from this package.
