#!/usr/bin/env python3
"""Package the deployer with the already-built Linux probes (no extra compilation)."""
import json
import os
from pathlib import Path
import tarfile

source = Path("source")
artifacts = json.loads((source / "dist/artifacts.json").read_text())
with tarfile.open(f"kite-deployer_{os.environ['TAG']}.tar.gz", "w:gz") as package:
    for name in ("deploy-server.sh", "deploy-ssh-gate.sh", "deploy-server-install.sh", "deploy-version"):
        package.add(source / "scripts" / name, arcname=name)
    for arch in ("amd64", "arm64"):
        matches = [a for a in artifacts if a.get("type") == "Binary" and a.get("extra", {}).get("ID") == "deploy-probe" and a.get("goos") == "linux" and a.get("goarch") == arch]
        if len(matches) != 1:
            raise SystemExit(f"expected one deploy-probe artifact for linux/{arch}")
        path = Path(matches[0]["path"])
        if not path.is_absolute():
            path = source / path
        if not path.resolve().is_relative_to((source / "dist").resolve()):
            raise SystemExit("probe artifact must be within source/dist")
        package.add(path, arcname=f"linux/{arch}/kite-probe")
