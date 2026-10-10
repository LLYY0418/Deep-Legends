"""Report R266 simulation separately from measured coverage of a supplied .gz export."""
import argparse
from datetime import datetime, timezone
import gzip
import json
import math
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPORTS = ROOT / "docs/history/reports/r266"
INPUTS = REPORTS / "user-logs"
parser = argparse.ArgumentParser()
parser.add_argument("--input", type=Path)
parser.add_argument("--output", type=Path, default=REPORTS / "log-coverage.json")
args = parser.parse_args()
total, minutes, score_count = 10_600_000, 37, 694
composition = {"pro_players": (13, 22_000), "refresh_succeeded": (6, 36_600),
               "objective_badge_state": (32, 23_000), "r99_read_probe": (40, 16_000)}
max_large = sum(count * size for count, size in composition.values())
other_total = total * .23
simulations = []
for ratio in (.25, .5, .75, 1):
    for matches_per_load in (1, 30):
        loads = math.ceil(score_count / matches_per_load)
        # 350 bytes includes normal diagnostic metadata and representative badge
        # counts. Special rows use their full hard cap; untouched bytes remain.
        new_score = loads * 350
        new_large = 13 * 350 + (6+32+40) * 2048
        unchanged = max(0, other_total - max_large * ratio)
        after = new_score + new_large + unchanged
        simulations.append({"large_event_mean_fraction_of_max": ratio, "matches_per_overview": matches_per_load,
                            "overview_summaries": loads, "new_bytes_per_37_minutes": round(after),
                            "coverage_minutes": round(total / after * minutes, 1),
                            "simulated_only": True, "acceptance_closed": False})
result = {"target_hours": 3, "simulation_label": "模拟，非实测", "source": "user-provided event composition",
          "baseline_bytes": total, "baseline_minutes": minutes, "score_byte_fraction": .77,
          "score_events": score_count, "score_max_bytes": 16_900,
          "large_events": {name: {"count": count, "max_bytes": size} for name, (count, size) in composition.items()},
          "assumptions": ["MB/KB treated as decimal; user sizes and 37 minutes are approximate",
                          "max size is not mean size; mean fractions are explicit sensitivity scenarios",
                          "representative summary is 350 bytes; special rows conservatively use the entire 2KiB cap",
                          "unchanged event bytes remain; successful score summaries are modelled without failure rows",
                          "new riot_request volume is unknown and is not inferred from the old export",
                          "simulation proves feasibility only; it cannot close the three-hour acceptance"],
          "模拟估算值": simulations, "实测值": "待用户提供日志", "measured_acceptance_closed": False}
if args.input:
    file = args.input.resolve()
    if INPUTS.resolve() not in file.parents or file.suffix != ".gz":
        raise SystemExit("Use a compressed .gz export inside docs/history/reports/r266/user-logs/.")
    timestamps, counts, bytes_read, invalid = [], {}, 0, 0
    with gzip.open(file, "rt", encoding="utf8", errors="replace") as stream:
        for line in stream:
            bytes_read += len(line.encode("utf8"))
            try:
                event = json.loads(line)
                name = event.get("event", "unknown")
                # Event names and numeric counts are retained; payloads and
                # identity/credential values are never written to the report.
                if not isinstance(name, str) or not re.fullmatch(r"[a-z][a-z0-9_]{0,79}", name):
                    name = "unknown"
                counts[name] = counts.get(name, 0)+1
                stamp = event.get("time")
                if isinstance(stamp, (int, float)):
                    timestamps.append(stamp/1000 if stamp > 100_000_000_000 else stamp)
                elif isinstance(stamp, str):
                    timestamps.append(datetime.fromisoformat(stamp.replace("Z", "+00:00")).timestamp())
            except (ValueError, TypeError, AttributeError):
                invalid += 1
    if not timestamps:
        raise SystemExit("No valid diagnostic timestamps found; no measured acceptance can be recorded.")
    seconds = max(timestamps)-min(timestamps)
    result["实测值"] = {"input": file.name, "uncompressed_bytes": bytes_read, "events": counts, "unparsed_lines": invalid,
                         "coverage_hours": round(seconds/3600, 4), "first_utc": datetime.fromtimestamp(min(timestamps), timezone.utc).isoformat(),
                         "last_utc": datetime.fromtimestamp(max(timestamps), timezone.utc).isoformat()}
    result["measured_acceptance_closed"] = seconds >= 3*3600
args.output.parent.mkdir(parents=True, exist_ok=True)
args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2)+"\n")
print(json.dumps({"simulation": "模拟，非实测", "实测值": result["实测值"], "closed": result["measured_acceptance_closed"]}, ensure_ascii=False))
