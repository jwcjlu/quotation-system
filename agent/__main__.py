"""python -m agent"""
from __future__ import annotations

import logging
import sys

from .app import AgentApp
from .config import Config


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    try:
        cfg = Config.from_env()
    except ValueError as e:
        print(e, file=sys.stderr)
        sys.exit(1)
    app = AgentApp(cfg)
    app.run()


if __name__ == "__main__":
    main()
