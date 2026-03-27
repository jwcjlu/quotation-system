"""python -m agent"""
from __future__ import annotations

import sys

from .app import AgentApp
from .config import Config
from .log_setup import configure_logging


def main() -> None:
    try:
        cfg = Config.from_env()
    except ValueError as e:
        print(e, file=sys.stderr)
        sys.exit(1)
    configure_logging(cfg.log_dir, cfg.log_level)
    app = AgentApp(cfg)
    app.run()


if __name__ == "__main__":
    main()
