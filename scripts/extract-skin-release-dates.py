#!/usr/bin/env python3
"""Extract verified skin.release dates from a locally downloaded Meraki dataset.
Usage: python3 scripts/extract-skin-release-dates.py SOURCE.json YYYY-MM-DD
The source is intentionally not downloaded by this script. Inspect coverage first.
"""
import datetime
import hashlib
import json
import pathlib
import sys

source = pathlib.Path(sys.argv[1]).read_bytes()
retrieved = datetime.date.fromisoformat(sys.argv[2]).isoformat()
dates = {}
for champion in json.loads(source).values():
    for skin in champion.get("skins", []):
        release = skin.get("release", "")
        try:
            date = datetime.date.fromisoformat(release).isoformat()
        except (ValueError, TypeError):
            continue
        if skin.get("isBase", False):
            continue
        dates[str(skin["id"])] = date
if len(dates) < 1000:
    raise ValueError("Unexpected source coverage; refusing to replace release snapshot")
result = {
    "source": "https://cdn.merakianalytics.com/riot/lol/resources/latest/en-US/champions.json",
    "retrievedAt": retrieved,
    "sourceSHA256": hashlib.sha256(source).hexdigest(),
    "latestReleaseDate": max(dates.values()),
    "dates": dict(sorted(dates.items(), key=lambda pair: int(pair[0]))),
}
target = pathlib.Path(__file__).resolve().parents[1] / "data/skin_release_dates.json"
target.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
print(f"{len(dates)} verified dates; latest covered release {max(dates.values())}")
