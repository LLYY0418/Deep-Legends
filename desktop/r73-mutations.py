from pathlib import Path
import shutil,tempfile,subprocess,os
root=Path(__file__).resolve().parents[1]; clone=Path(tempfile.mkdtemp(prefix='r73-mutation-',dir='/private/tmp'))
for f in root.glob('*.go'): shutil.copy2(f,clone/f.name)
for f in ['go.mod','go.sum','prestige_chromas.json']: shutil.copy2(root/f,clone/f)
for d in ['web','data']: shutil.copytree(root/d,clone/d)
env=dict(os.environ,GOCACHE=str(root/'.gocache'),GOTMPDIR='/private/tmp')
mutants=[
 ('M1','pro_players.go','allowed := sourceTeam.ID == team.OPGGID','allowed := true','TestR73TeamGate'),
 ('M2','pro_players.go','k = append([]string{"puuid:" + raw.PUUID}, k...)','k = append(k, "puuid:" + raw.PUUID)','TestR73StableIdentityAnchor'),
 ('M3','pro_players.go','time.Since(at) > proAccountDormantAfter','time.Since(at) < proAccountDormantAfter','TestR73DormantPrimary'),
 ('M4','pro_players.go','if a.Dormant != b.Dormant {\n\t\treturn !a.Dormant\n\t}','', 'TestR73DormantPrimary'),
 ('M5','pro_players_ladder.go','strings.ContainsAny(account.GameName, "- ") || ','','TestR73AmbiguousLadderNoRequest'),
 ('M6','pro_players.go','type proAccount struct {','type proAccount struct {\n PUUID string `json:"puuid"`','TestR73StableIdentityAnchor'),
]
for label,file,old,new,test in mutants:
 original=(root/file).read_text();assert old in original,(label,'target missing');(clone/file).write_text(original.replace(old,new,1))
 result=subprocess.run(['go','test','-run','^'+test+'$', '.'],cwd=clone,env=env,capture_output=True,text=True)
 # Compile errors are not mutation kills.
 assert result.returncode and '--- FAIL: '+test in result.stdout,(label,result.stdout,result.stderr)
 print(label,'KILLED',flush=True);(clone/file).write_text(original)
print('All Go mutants killed; real copied workspace:',clone)
# NSIS guards read a physical copied fixture too, never the live include.
(clone/'desktop/nsis').mkdir(parents=True)
shutil.copy2(root/'desktop/installer-nsh.test.cjs',clone/'desktop/installer-nsh.test.cjs')
installer=(root/'desktop/nsis/installer.nsh').read_text()
for label,old,new in [
 ('N-1','SendMessage $DLLaunch ${WM_SETFONT} $DLBodyFont 1',''),
 ('N-2','$HWNDPARENT 1028','$HWNDPARENT 9999'),
 ('N-3','  !define MUI_PAGE_CUSTOMFUNCTION_SHOW DLInstFilesShow',''),
 ('N-4','Function DLDirectoryLeave','GetDlgItem $0 $HWNDPARENT 1037\nShowWindow $0 ${SW_SHOW}\nFunction DLDirectoryLeave'),
 ('N-5','0x41A4D9','0xD9A441')]:
 assert old in installer
 (clone/'desktop/nsis/installer.nsh').write_text(installer.replace(old,new,1))
 result=subprocess.run(['node','--test','--test-name-pattern=^'+label+' ', 'desktop/installer-nsh.test.cjs'],cwd=clone,capture_output=True,text=True)
 assert result.returncode and ('not ok' in result.stdout or '✖' in result.stdout),(label,result.stdout,result.stderr)
 print(label,'KILLED',flush=True)
