"""Summarize supplied R258 JSONL evidence; never infer a physical pass."""
import argparse, json, math, pathlib

def observed(value):
    return type(value) in (int, float) and math.isfinite(value) and value >= 0

def summarize(paths, akari_version, server):
    rows=[]; issues=[]; groups={}
    for path in paths:
        for n,line in enumerate(pathlib.Path(path).read_text(encoding='utf-8-sig').splitlines(),1):
            if not line.strip(): continue
            try: row=json.loads(line)
            except json.JSONDecodeError as error: raise ValueError(f'{path}:{n}: invalid JSONL') from error
            if isinstance(row,dict):
                fingerprint=row.get('build_fingerprint'); run=row.get('run_id')
                group=(str(fingerprint or ''),str(run or pathlib.Path(path).resolve()))
                info=groups.setdefault(group,{'build_fingerprint':fingerprint,'run_id':run,'input_files':set()})
                info['input_files'].add(str(path))
                rows.append((group,row,f'{path}:{n}'))
    cold={};close={};calls={}
    for group,row,location in rows:
        event=row.get('event')
        if event not in ('client_cold_launch_timeline','client_close_timeline','champselect_request_client'): continue
        if not row.get('build_fingerprint') or not row.get('run_id'):
            issues.append(f'{location}: missing build_fingerprint/run_id; isolated by input file where run_id is absent')
        if event=='client_cold_launch_timeline':
            epoch=row.get('process_at')
            if not observed(epoch):
                issues.append(f'{location}: missing/invalid process_at'); continue
            identity=(*group,epoch); revision=row.get('timeline_revision')
            if not observed(revision): issues.append(f'{location}: missing/invalid timeline_revision; recording order requires review')
            rank=(revision if observed(revision) else -1,row.get('log_seq',0))
            old=cold.get(identity)
            if old is None or rank>=old[0]: cold[identity]=(rank,row)
        elif event=='client_close_timeline':
            epoch=row.get('shutdown_signal_at')
            if not observed(epoch):
                issues.append(f'{location}: missing/invalid shutdown_signal_at'); continue
            identity=(*group,epoch); sequence=row.get('log_seq')
            if not observed(sequence): issues.append(f'{location}: missing/invalid close log_seq; recording order requires review')
            rank=sequence if observed(sequence) else -1
            if identity not in close or rank>=close[identity][0]: close[identity]=(rank,row)
        elif event=='champselect_request_client':
            identity=(*group,row.get('request_id'),row.get('started_at'),row.get('completed_at'))
            if not all(observed(value) for value in identity[2:]):
                issues.append(f'{location}: incomplete champselect request identity; no timestamp-based deduplication')
                identity=(*group,'incomplete',row.get('log_seq',location))
            calls[identity]=row
    def context(group):
        info=groups[group]
        return {**info,'input_files':sorted(info['input_files'])}
    def zero(value): return value==0 if observed(value) else None
    def delta(row, end, start):
        values=[row.get(end),row.get(start)]
        return values[0]-values[1] if all(observed(v) for v in values) else None
    starts=[]
    for (*group,epoch),(_,row) in sorted(cold.items()):
        group=tuple(group)
        header=delta(row,'self_tab_header_ms','summoner_ready_ms')
        matches=delta(row,'matches_card_ms','summoner_ready_ms')
        source=row.get('matches_card_source');limit=500 if source=='snapshot' else 2000 if source=='network' else None
        first_change=delta(row,'ui_first_change_ms','self_tab_header_ms')
        starts.append({**context(group),'process_at':epoch,'timeline_revision':row.get('timeline_revision'),
          'header_after_summoner_ms':header,'matches_after_summoner_ms':matches,'matches_source':source,
          'log_checks':{'overlay_zero':zero(row.get('overlay_shown')),'first_change_is_header':None if first_change is None else first_change==0,
            'header_le_300ms':None if header is None else 0<=header<=300,'matches_within_target':None if matches is None or limit is None else 0<=matches<=limit},
          'sgp_bytes_first_screen':row.get('sgp_bytes_first_screen'), 'lcu_requests_first_3s':row.get('lcu_requests_first_3s'),
          'lcu_window_elapsed':row.get('lcu_requests_window_elapsed'), 'browser_queued_requests_first_3s':row.get('browser_queued_requests_first_3s'),
          'browser_window_elapsed':row.get('browser_requests_window_elapsed'),'browser_resource_count_first_3s':row.get('browser_resource_count_first_3s'),
          'browser_queue_measurement':row.get('browser_queue_measurement')})
    closes=[]
    for (*group,epoch),(_,row) in sorted(close.items()):
        removed=row.get('tab_removed_ms')
        closes.append({**context(tuple(group)),'shutdown_signal_at':epoch,'tab_removed_ms':removed,
          'restored_flip':row.get('restored_flip'),'overlay_zero':zero(row.get('overlay_shown')),
          'tab_removed_le_300ms':removed<=300 if observed(removed) else None})
    call_runs=[]
    for group in sorted({identity[:2] for identity in calls}):
        samples=[row for identity,row in calls.items() if identity[:2]==group]
        measured=[row['response_bytes'] for row in samples if observed(row.get('response_bytes'))]
        call_runs.append({**context(group),'observed_calls':len(samples),'bytes_samples':len(measured),
          'mean_response_body_bytes':sum(measured)/len(measured) if measured else None})
    return {'scope':'Supplied diagnostics only; recording and physical environment review required.',
        'acceptance_status':'awaiting_windows_recording_review','environment':{'akari_version_user_reported':akari_version,'server_user_reported':server},
        'cold_launches':starts,'client_closes':closes,'champselect_state':{'runs':call_runs},'diagnostic_issues':issues,
        'limitations':['ResourceTiming counts completed local resources; late completions amend the frame. Cross-check pending requests and diagnostic delivery in the Windows trace.',
                       'SGP bytes are completed response bodies through first DOM card, including retries; not wire-level bytes.',
                       'LCU count covers submitted JSON and byte HTTP requests, including discovery; no WebSocket frame count.',
                       'A source-derived estimate, synthetic run or missing metric never constitutes physical acceptance.']}

def main():
    parser=argparse.ArgumentParser();parser.add_argument('diagnostics',nargs='+');parser.add_argument('--akari-version',required=True);parser.add_argument('--server',required=True);parser.add_argument('--output',required=True)
    args=parser.parse_args();output=pathlib.Path(args.output)
    if output.exists(): raise SystemExit('Output already exists; choose a new review result path.')
    output.parent.mkdir(parents=True,exist_ok=True);output.write_text(json.dumps(summarize(args.diagnostics,args.akari_version,args.server),ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
if __name__=='__main__':main()
