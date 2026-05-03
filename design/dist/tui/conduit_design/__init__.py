"""Conduit Design — Textual/Rich theme loader.

Three modes are bundled: ``dark`` (default), ``light``, and ``hc``
(high-contrast, WCAG AAA 7:1). Source of truth is ``design/tokens.yaml``
in the conduit repo.
"""

from __future__ import annotations

import json
from importlib import resources
from typing import Literal

Mode = Literal["dark", "light", "hc"]

__all__ = ["Mode", "load_theme"]


def load_theme(mode: Mode = "dark") -> dict:
    """Return the parsed Textual/Rich theme dict for ``mode``."""
    if mode not in ("dark", "light", "hc"):
        raise ValueError(f"unknown mode {mode!r}; expected dark, light, or hc")
    name = f"theme-{mode}.json"
    with resources.files(__package__).joinpath(name).open("r", encoding="utf-8") as fh:
        return json.load(fh)
