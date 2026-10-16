"""Target-bound R271 proofs. Infrastructure failures never kill a mutant."""
import json,re

INFRA = re.compile(r'ETIMEDOUT|ENOENT|ParserError|SyntaxError|Cannot find module|ReferenceError|TypeError|spawn error|WAIT_STALLED|WAIT_HARD_LIMIT')

def verify(returncode, log, negative=False, target=None, assertion=None):
    if not negative:
        return returncode == 0
    if returncode != 1 or INFRA.search(log) or not target or not assertion:
        return False
    # Native guards carry the original assertion and its real source stack.
    if target == 'native':
        return len(re.findall(r'(?m)^AssertionError\b',log)) == 1 and assertion in log and 'scripts/r266-browser.cjs:' in log
    failures = re.split(r'(?m)^not ok \d+ - ', log)[1:]
    return bool(failures) and all(
        re.fullmatch(target, block.splitlines()[0]) and ' '.join(assertion.split()) in ' '.join(re.sub(r'(?m)^\s*\|\s?', '', block).split()) and
        ('ERR_ASSERTION' in block or 'ASSERTION FAILED:' in block)
        for block in failures)

def failure_detail(name, returncode, log, evidence, target=None, assertion=None):
    locations = re.findall(r"location: ['\"]([^'\"]+\.(?:cjs|js)):(\d+):\d+", log)
    if not locations:
        locations = re.findall(r'([^\s()]*?/(?:backend|desktop|scripts)/[^\s():]+\.(?:cjs|js)):(\d+):\d+',log)
    file, line = locations[0] if locations else ('scripts/r271-mutations.py', None)
    file = re.sub(r'^.*?/(?=(?:backend|desktop|scripts)/)', '', file)
    stages = re.findall(r'(?m)^\s*(R271_STAGE [\w.-]+ \d+ms)\s*$', log)
    code = next((code for code in ('ETIMEDOUT','ENOENT','WAIT_STALLED','WAIT_HARD_LIMIT') if code in log), None)
    lines=log.splitlines(); original_assertion=None
    for index, text in enumerate(lines):
        found=re.match(r'^\s*\|\s*(ASSERTION FAILED:.*)',text)
        if found:
            parts=[found.group(1)]
            for following in lines[index+1:]:
                continuation=re.match(r'^\s*\|\s*(.+)',following)
                if not continuation: break
                parts.append(continuation.group(1))
            original_assertion=' '.join(parts); break
    message = code or original_assertion or next((s.strip() for s in lines if s.lstrip().startswith('AssertionError')), 'mutation proof did not match its target')
    message = re.sub(r'RGAPI-[A-Za-z0-9-]+', '[riot-key]', message)
    message = re.sub(r'(?i)([A-Z]:[\\/]+Users[\\/]+|/Users/|/home/)[^\\/\r\n\"\']+',r'\1[user]',message)
    evidence = '/'.join(str(evidence).replace('\\','/').split('/')[-2:])
    return dict(schemaVersion=1,name=name,file=file,line=int(line) if line else None,exitCode=returncode,
                errorCode=code,message=message[:400],lastStage=stages[-1] if stages else None,
                evidence=str(evidence),target=target,assertion=assertion)
