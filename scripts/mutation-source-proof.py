"""Checkout integrity proof shared by overlay/disposable mutation runners."""
import hashlib, subprocess
from pathlib import Path

def snapshot(root):
    files=subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','-z'],cwd=root).decode().split('\0')
    return {name:hashlib.sha256((Path(root)/name).read_bytes()).hexdigest()
            for name in files if name and (Path(root)/name).is_file()}

def unchanged(root, before):
    after=snapshot(root)
    changed=sorted(name for name in before.keys()|after.keys() if before.get(name)!=after.get(name))
    assert not changed,'mutation runner changed checkout source: '+', '.join(changed)
    print('MUTATION_CHECKOUT_UNCHANGED',flush=True)
