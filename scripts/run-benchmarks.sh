#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
report_dir="docs/r115-validation/bench"
mkdir -p "$report_dir"
report_base="$report_dir/$(date +%Y%m%d-%H%M%S)"
GOCACHE="${GOCACHE:-$PWD/.gocache}" go test -run '^$' -bench '^BenchmarkR115' -benchmem -benchtime=200ms -count=5 . > "$report_base.txt"
python3 - "$report_base.txt" "${1:-}" <<'PY'
from pathlib import Path
from collections import defaultdict
from statistics import median
import re,sys
pattern=re.compile(r'^(Benchmark\S+)\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',re.M)
def parse(path):
 rows=defaultdict(list)
 for name,ns,size,alloc in pattern.findall(Path(path).read_text()): rows[name].append(tuple(map(float,(ns,size,alloc))))
 return {name:tuple(median(x[i] for x in values) for i in range(3)) for name,values in rows.items()}
current=parse(sys.argv[1]);baseline=parse(sys.argv[2]) if sys.argv[2] else {}
lines=['# R115 offline benchmark medians','','Five samples, 200 ms each. No upstream requests. Compare only on the same machine under similar load.','','| Benchmark | ns/op | B/op | allocs/op | time vs baseline |','|---|---:|---:|---:|---:|']
for name,values in current.items():
 prior=baseline.get(name);delta=f'{(values[0]/prior[0]-1)*100:+.1f}%' if prior and prior[0] else '—'
 lines.append('| '+name+' | '+' | '.join(f'{v:g}' for v in values)+' | '+delta+' |')
Path(sys.argv[1]).with_suffix('.md').write_text('\n'.join(lines)+'\n')
print(Path(sys.argv[1]).with_suffix('.md'))
PY
