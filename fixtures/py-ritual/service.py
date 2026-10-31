"""Fixture: imports two of four declared packages and one undeclared module.

`yaml` resolves to pyyaml, `rich` is declared directly. `ghost_import` exists
nowhere, which is the undeclared case. `requests` and `jinja2` are declared and
never imported, so they are phantom.
"""

from __future__ import annotations

import json
import os

import ghost_import
import yaml
from rich.console import Console

console = Console()


def load(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        data = yaml.safe_load(fh)
    console.print(json.dumps(data, indent=2))
    return ghost_import.decorate(data)


if __name__ == "__main__":
    load(os.path.join(os.path.dirname(__file__), "config.yaml"))
