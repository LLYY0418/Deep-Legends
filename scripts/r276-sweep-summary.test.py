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
        command=['go','test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+'|'.join(module.HEXDATA_TESTS)+')$','./backend']
        self.stress=dict(platform='win32',sha='fixture-sha',count=200,cpu=[1,2,4],command=command,exitCode=0,rows=[dict(test=t,passed=600,failed=0,samples=600) for t in module.HEXDATA_TESTS])
        self.events=[dict(Action='pass',Test=t,Elapsed=0.01) for t in module.HEXDATA_TESTS for _ in range(600)]
    def result(self):
        (self.folder/'samples.json').write_text(json.dumps(self.rows));(self.folder/'0/hexdata/stress-summary.json').write_text(json.dumps(self.stress));(self.folder/'0/hexdata/stress.jsonl').write_text('\n'.join(json.dumps(e) for e in self.events)+'\n')
        return module.summarize(self.root,self.jobs,'fixture/repo',1,inventory={},catalog=[],plan=[('windows-hexdata',0,1)])
    def test_complete_stress_and_metrics(self):
        r=self.result();self.assertTrue(r['success']);self.assertEqual(r['hexdataStress'][0]['rows'][0]['samples'],600);self.assertEqual(r['metrics']['windows-hexdata/'+module.HEXDATA_TESTS[0]]['count'],600)
    def test_missing_raw_repetition_rejected(self):
        self.events.pop();self.assertFalse(self.result()['success'])
    def test_missing_test_rejected(self):
        self.stress['rows'].pop();self.assertFalse(self.result()['success'])
    def test_wrong_count_cpu_or_race_rejected(self):
        self.stress['command'].remove('-race');self.assertFalse(self.result()['success'])
    def test_failed_stress_rejected(self):
        self.stress['rows'][0].update(passed=599,failed=1);self.assertFalse(self.result()['success'])
    def test_raw_failure_cannot_hide_in_clean_summary(self):
        self.events.append(dict(Action='fail',Test=module.HEXDATA_TESTS[0]));self.assertFalse(self.result()['success'])
    def test_wrong_sha_rejected(self):
        self.stress['sha']='other';self.assertFalse(self.result()['success'])
if __name__=='__main__':unittest.main()
