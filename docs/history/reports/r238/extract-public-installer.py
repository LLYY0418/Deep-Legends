"""Read-only installer extraction for auditing actual Windows CI/draft bytes.
The derived receipt describes extracted bytes; it is not an original build receipt.
"""
import hashlib
import json
import pathlib
import shutil
import struct
import subprocess
import sys

setup, output, sevenzip, fingerprint = sys.argv[1:]
setup, output = pathlib.Path(setup).resolve(), pathlib.Path(output).resolve()
if output.exists():
    raise SystemExit('Audit output already exists; select a new directory')
output.mkdir(parents=True)
raw = setup.read_bytes()
pe32 = []
offset = 0
while True:
    start = raw.find(b'MZ', offset)
    if start < 0:
        break
    offset = start + 2
    if start + 64 >= len(raw):
        continue
    header = start + struct.unpack_from('<I', raw, start + 60)[0]
    if header > start + 1048576 or header + 24 >= len(raw) or raw[header:header + 4] != b'PE\0\0':
        continue
    machine, sections = struct.unpack_from('<HH', raw, header + 4)
    if machine == 0x14c and 0 < sections <= 40 and struct.unpack_from('<H', raw, header + 20)[0] == 224:
        pe32.append(start)
if len(pe32) != 1:
    raise SystemExit(f'Ambiguous embedded NSIS PE offsets: {pe32}')
nsis = output / 'embedded-nsis.exe'
nsis.write_bytes(raw[pe32[0]:])
subprocess.run([sevenzip, 'x', str(nsis), '-o' + str(output / 'nsis'), '$PLUGINSDIR/app-64.7z'], check=True)
subprocess.run([sevenzip, 'x', str(output / 'nsis/$PLUGINSDIR/app-64.7z'), '-o' + str(output / 'win-unpacked')], check=True)
archive = output / 'win-unpacked/resources/app.asar'
backend = output / 'win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe'
version = subprocess.check_output(['node', '-e', "const a=require('./desktop/node_modules/@electron/asar'); console.log(JSON.parse(a.extractFile(process.argv[1],'package.json')).version)", str(archive)], text=True).strip()
subprocess.run(['node', 'desktop/verify-build-fingerprint.cjs', str(backend), fingerprint], check=True)
shutil.copyfile(setup, output / setup.name)
receipt = {'schema': 1, 'version': version, 'fingerprint': fingerprint, 'mode': 'public',
           'backendSHA256': hashlib.sha256(backend.read_bytes()).hexdigest(),
           'assets': {setup.name: hashlib.sha256(raw).hexdigest()},
           'provenance': 'derived from actual extracted installer bytes, not original build receipt',
           'embedded_nsis_offset': pe32[0], 'setup_size': len(raw)}
(output / 'release-build.json').write_text(json.dumps(receipt, indent=2) + '\n')
print(json.dumps(receipt))
