"""Synthetic artifact regressions; no CI/network/real runner timings claimed."""
import importlib.util, json, tempfile, unittest
from pathlib import Path
spec=importlib.util.spec_from_file_location('summary',Path(__file__).with_name('r276-sweep-summary.py'));module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class SummaryTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.root=Path(self.temp.name);self.folder=self.root/'job';(self.folder/'0').mkdir(parents=True)
        self.rows=[dict(kind='windows-renderers',index=0,iteration=0,label='renderers',job='windows-renderers-0',run='1',sha='fixture-sha',duration_ms=100,exitCode=0,signatures=[])]
        self.jobs=[dict(jobs=[dict(name='windows-renderers-0',conclusion='success',html_url='https://github.com/fixture/repo/actions/runs/1/job/2')])]
        self.timing=dict(success=True,platform='win32',summary=dict(duration_ms=100),files=[dict(file='desktop/fixture.test.cjs',duration_ms=50)])
    def result(self):
        (self.folder/'samples.json').write_text(json.dumps(self.rows));(self.folder/'0/renderer-timings.json').write_text(json.dumps(self.timing))
        return module.summarize(self.root,self.jobs,'fixture/repo',1,inventory={'win32':['desktop/fixture.test.cjs'],'linux':[]},catalog=[],plan=[('windows-renderers',0,1)])
    def test_clean_and_metrics(self):
        result=self.result();self.assertTrue(result['success']);self.assertEqual(result['metrics']['win32/suite']['median_ms'],100);self.assertTrue(result['sampleRecords'][0]['jobLink'].endswith('/job/2'))
    def test_duplicate_iteration_rejected(self):
        self.rows.append(dict(self.rows[0]));self.assertFalse(self.result()['success'])
    def test_missing_job_rejected(self):
        self.jobs=[];self.assertFalse(self.result()['success'])
    def test_missing_renderer_inventory_rejected(self):
        self.timing['files']=[];self.assertFalse(self.result()['success'])
    def test_budget_rejected(self):
        self.timing['files'][0]['duration_ms']=120001;self.assertFalse(self.result()['success'])
    def test_windows_suite_boundary_preserved(self):
        self.timing['summary']['duration_ms']=240000;self.assertTrue(self.result()['success'])
        self.timing['summary']['duration_ms']=240001;self.assertFalse(self.result()['success'])
    def test_linux_budget_boundaries(self):
        kind='linux-renderers';name=kind+'-0'
        self.rows=[dict(kind=kind,index=0,iteration=0,label=label,job=name,run='1',sha='fixture-sha',duration_ms=100,exitCode=0,signatures=[]) for label in ['calibration-before','renderers','calibration-after','budget']]
        self.jobs=[dict(jobs=[dict(name=name,conclusion='success',html_url='https://github.com/fixture/repo/actions/runs/1/job/2')])]
        self.timing.update(platform='linux');self.timing['files'][0]['duration_ms']=100000
        (self.folder/'0/budget.json').write_text(json.dumps(dict(ok=True,normalized_ms=190000)))
        for duration,file_time,want in [(270000,100000,True),(270001,100000,False),(270000,100001,False)]:
            with self.subTest(duration=duration,file=file_time):
                self.timing['summary']['duration_ms']=duration;self.timing['files'][0]['duration_ms']=file_time
                (self.folder/'samples.json').write_text(json.dumps(self.rows));(self.folder/'0/renderer-timings.json').write_text(json.dumps(self.timing))
                result=module.summarize(self.root,self.jobs,'fixture/repo',1,inventory={'linux':['desktop/fixture.test.cjs'],'win32':[]},catalog=[],plan=[(kind,0,1)])
                self.assertEqual(result['success'],want)
    def test_failure_signature_and_job_link(self):
        self.rows[0].update(exitCode=1,signatures=['fixture assertion']);result=self.result();self.assertFalse(result['success']);self.assertEqual(result['failures'][0]['count'],1);self.assertEqual(result['failures'][0]['fraction'],1);self.assertEqual(result['failures'][0]['jobLinks'],[self.jobs[0]['jobs'][0]['html_url']])
    def test_nonfinite_metric_rejected(self):
        self.timing['summary']['duration_ms']=float('nan');self.assertFalse(self.result()['success'])
    def test_missing_sha_rejected(self):
        self.rows[0]['sha']=None;self.assertFalse(self.result()['success'])
class HexdataSummaryTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.root=Path(self.temp.name);self.folder=self.root/'job';(self.folder/'0/hexdata').mkdir(parents=True)
        self.rows=[dict(kind='windows-hexdata',index=0,iteration=0,label='hexdata',job='windows-hexdata-0',run='1',sha='fixture-sha',duration_ms=100,exitCode=0,signatures=[])]
        self.jobs=[dict(jobs=[dict(name='windows-hexdata-0',conclusion='success',html_url='https://github.com/fixture/repo/actions/runs/1/job/2')])]
        self.stress=dict(platform='win32',sha='fixture-sha',count=200,cpu=[1,2,4],strategy='per-test-default-timeout',exitCode=0,rows=[dict(test=t,passed=600,failed=0,samples=600) for t in module.HEXDATA_TESTS],invocations=[])
        self.parts=[]
        for i,t in enumerate(module.HEXDATA_TESTS):
            command=['go','test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+t+')$','./backend']
            self.stress['invocations'].append(dict(test=t,command=command,exitCode=0,error=None,raw='invocation-'+str(i)+'.jsonl',duration_ms=100))
            self.parts.append([dict(Action='pass',Package='lol-loot-assistant/backend',Test=t,Elapsed=0.01) for _ in range(600)]+[dict(Action='pass',Package='lol-loot-assistant/backend',Elapsed=6)])
        self.events=[e for part in self.parts for e in part]
    def result(self):
        (self.folder/'samples.json').write_text(json.dumps(self.rows));(self.folder/'0/hexdata/stress-summary.json').write_text(json.dumps(self.stress));(self.folder/'0/hexdata/stress.jsonl').write_text('\n'.join(json.dumps(e) for e in self.events)+'\n')
        for i,part in enumerate(self.parts):(self.folder/'0/hexdata'/('invocation-'+str(i)+'.jsonl')).write_text('\n'.join(json.dumps(e) for e in part)+'\n')
        return module.summarize(self.root,self.jobs,'fixture/repo',1,inventory={},catalog=[],plan=[('windows-hexdata',0,1)])
    def test_complete_stress_and_metrics(self):
        r=self.result();self.assertTrue(r['success']);self.assertEqual(r['hexdataStress'][0]['rows'][0]['samples'],600);self.assertEqual(r['metrics']['windows-hexdata/'+module.HEXDATA_TESTS[0]]['count'],600)
    def test_missing_raw_repetition_rejected(self):
        self.events.pop();self.assertFalse(self.result()['success'])
    def test_missing_test_rejected(self):
        self.stress['rows'].pop();self.assertFalse(self.result()['success'])
    def test_wrong_count_cpu_or_race_rejected(self):
        self.stress['invocations'][0]['command'].remove('-race');self.assertFalse(self.result()['success'])
    def test_failed_stress_rejected(self):
        self.stress['rows'][0].update(passed=599,failed=1);self.assertFalse(self.result()['success'])
    def test_raw_failure_cannot_hide_in_clean_summary(self):
        self.events.append(dict(Action='fail',Test=module.HEXDATA_TESTS[0]));self.assertFalse(self.result()['success'])
    def test_missing_invocation_rejected(self):
        self.stress['invocations'].pop();self.assertFalse(self.result()['success'])
    def test_grouped_package_fuse_rejected(self):
        self.stress['invocations'][0]['command'][-2]='^('+'|'.join(module.HEXDATA_TESTS)+')$';self.assertFalse(self.result()['success'])
    def test_invocation_failure_cannot_hide_in_combined(self):
        self.parts[0].append(dict(Action='fail'));self.assertFalse(self.result()['success'])
    def test_ten_minute_fuse_cannot_be_extended(self):
        self.stress['invocations'][0]['command'].insert(2,'-timeout=20m');self.assertFalse(self.result()['success'])
    def test_wrong_package_pass_rejected(self):
        self.parts[0][-1]['Package']='other';self.assertFalse(self.result()['success'])
    def test_partial_package_pass_rejected(self):
        self.parts[0].pop();self.assertFalse(self.result()['success'])
    def test_wrong_sha_rejected(self):
        self.stress['sha']='other';self.assertFalse(self.result()['success'])

class LegacyRetryEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.root=Path(self.temp.name)
        for i,event in enumerate(['push','workflow_dispatch']):
            d=self.root/str(i);(d/'upgrade').mkdir(parents=True)
            (d/'acceptance-run.json').write_text(json.dumps(dict(id=i,head_sha='same',conclusion='success',run_attempt=1,event=event,html_url='https://fixture/'+str(i))))
            (d/'upgrade/r276-legacy-installer-retries.json').write_text(json.dumps(dict(total_retries=0,records=[])))
        self.d=self.root/'0/upgrade'
        self.record=dict(phase='published-legacy-setup',version='0.12.81',signature='invalid pointer found on stack + main.shellWndProc',retry_count=1,attempt_count=2,completed=True,first_exit_code=2,second_exit_code=0,first_stderr='C:\\fixture\\first-stderr.log',second_stderr='C:\\fixture\\second-stderr.log',evidence_json='C:\\fixture\\first-retry.json')
    def result(self):
        (self.d/'first-stderr.log').write_text('invalid pointer found on stack\nmain.shellWndProc')
        (self.d/'second-stderr.log').write_text('')
        (self.d/'first-retry.json').write_text(json.dumps(self.record))
        for name,exit_code in [('first',self.record['first_exit_code']),('second',self.record['second_exit_code'])]:
            (self.d/(name+'-result.json')).write_text(json.dumps(dict(version=self.record['version'],exit_code=exit_code,stderr_bytes=(self.d/(name+'-stderr.log')).stat().st_size)))
        (self.d/'r276-legacy-installer-retries.json').write_text(json.dumps(dict(total_retries=1,records=[self.record])))
        return module.upgrade_retry_evidence(self.root,'same')
    def test_counts_both_runs_without_rerunning_installer(self):
        data,errors=self.result();self.assertFalse(errors);self.assertEqual(data['total_retries'],1);self.assertEqual(len(data['runs']),2)
    def test_retry_limit_two_rejected(self):
        self.record['retry_count']=2;self.assertTrue(self.result()[1])
    def test_candidate_retry_rejected(self):
        self.record['version']='0.12.83';self.assertTrue(self.result()[1])
    def test_failed_second_attempt_rejected(self):
        self.record['second_exit_code']=2;self.assertTrue(self.result()[1])
    def test_missing_other_acceptance_run_rejected(self):
        (self.root/'1/acceptance-run.json').unlink();self.assertTrue(self.result()[1])
    def test_wrong_sha_rejected(self):
        self.result();self.assertTrue(module.upgrade_retry_evidence(self.root,'different')[1])

if __name__=='__main__':unittest.main()
