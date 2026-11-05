"""Development-only central test overlay for the staged network transaction slice."""
import json
from pathlib import Path

root = Path(__file__).resolve().parents[2]
replacements = {
    str(root / "network" / p.name): str(p)
    for p in (root / "tests/_cases/network").glob("*_test.go")
}
for p in (root / "network/_mutation").glob("*.go"):
    replacements[str(root / "network" / p.name)] = str(p)
target = root / "tests/data/network/_mutation_overlay.json"
target.write_text(json.dumps({"Replace": replacements}, indent=2) + "\n")
print(target)
