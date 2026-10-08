"""Local-only research paths; no product or CI test requires these inputs."""
import os
from pathlib import Path
EVIDENCE_ROOT = Path(os.environ.get('DEEP_LEGENDS_EVIDENCE_ROOT', Path(__file__).resolve().parent.parent / 'output/evidence')).resolve()
def evidence_path(*parts):
 return EVIDENCE_ROOT.joinpath(*parts)
def require_evidence(*parts):
 for relative in parts:
  path = evidence_path(relative)
  if not path.exists():
   raise SystemExit(f'Missing local evidence: {path}. Set DEEP_LEGENDS_EVIDENCE_ROOT to your existing report directory.')
