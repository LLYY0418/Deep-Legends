import importlib.util
import json
import pathlib
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('r258_evidence_summary',pathlib.Path(__file__).with_name('r258-evidence-summary.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

class EvidenceSummaryTest(unittest.TestCase):
    def report(self, *files):
        with tempfile.TemporaryDirectory() as directory:
            paths=[]
            for index,rows in enumerate(files):
                path=pathlib.Path(directory)/f'{index}.jsonl'
                path.write_text('\n'.join(json.dumps(row) for row in rows),encoding='utf-8');paths.append(path)
            return module.summarize(paths,'最新的','国服黑色玫瑰')

    def test_runs_and_builds_are_separate_and_overlapping_exports_deduplicate(self):
        row={'event':'champselect_request_client','build_fingerprint':'aaaa','run_id':'run-a',
             'request_id':1,'started_at':1000,'completed_at':1100,'response_bytes':100}
        other={**row,'run_id':'run-b','response_bytes':500}
        build={**row,'build_fingerprint':'bbbb','response_bytes':900}
        report=self.report([row,other,build],[row])
        runs=report['champselect_state']['runs']
        self.assertEqual([r['observed_calls'] for r in runs],[1,1,1])
        self.assertEqual([r['mean_response_body_bytes'] for r in runs],[100,500,900])
        self.assertEqual(len(runs[0]['input_files']),2)

    def test_missing_metrics_are_unknown_and_missing_identity_is_reported(self):
        report=self.report([{'event':'client_cold_launch_timeline','process_at':1000},
                            {'event':'client_close_timeline','shutdown_signal_at':2000},
                            {'event':'champselect_request_client','response_bytes':-1},
                            {'event':'champselect_request_client','response_bytes':-1}])
        checks=report['cold_launches'][0]['log_checks']
        self.assertTrue(all(value is None for value in checks.values()))
        self.assertIsNone(report['client_closes'][0]['overlay_zero'])
        self.assertIsNone(report['client_closes'][0]['tab_removed_le_300ms'])
        self.assertEqual(report['champselect_state']['runs'][0]['observed_calls'],2)
        self.assertIsNone(report['champselect_state']['runs'][0]['mean_response_body_bytes'])
        self.assertTrue(report['diagnostic_issues'])
        self.assertEqual(report['acceptance_status'],'awaiting_windows_recording_review')

    def test_highest_revision_and_close_sequence_survive_reversed_exports(self):
        base={'build_fingerprint':'aaaa','run_id':'run-a'}
        cold={**base,'event':'client_cold_launch_timeline','process_at':1000,'timeline_revision':2,
              'self_tab_header_ms':200,'summoner_ready_ms':100,'ui_first_change_ms':200,'overlay_shown':0}
        close={**base,'event':'client_close_timeline','shutdown_signal_at':2000,'log_seq':20,'tab_removed_ms':200,'overlay_shown':0}
        report=self.report([cold,close],[{**cold,'timeline_revision':1,'self_tab_header_ms':999},
                                       {**close,'log_seq':10,'tab_removed_ms':-1}])
        self.assertEqual(report['cold_launches'][0]['header_after_summoner_ms'],100)
        self.assertEqual(report['client_closes'][0]['tab_removed_ms'],200)

if __name__=='__main__':unittest.main()
