'use strict';
// Only separates independent legacy-upgrade scenarios in the CI-owned fixture.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
function resetLegacyPreferences(directory,label){
 if(!/^[0-9][0-9.-]{1,24}$/.test(label))throw Error('Invalid legacy fixture label');
 const root=fs.realpathSync(directory),data=path.join(root,'data'),evidence=path.join(root,'evidence');
 for(const folder of [data,evidence])if(fs.realpathSync(folder)!==folder)throw Error('Legacy fixture directories must stay inside their temporary root');
 const file=path.join(data,'ui-preferences.json'),archive=path.join(evidence,`r297-before-${label}-ui-preferences.json`);
 let info;try{info=fs.lstatSync(file)}catch(error){if(error.code!=='ENOENT')throw error}
 const record={label,hadAuthority:!!info};
 if(info){
  if(!info.isFile()||info.isSymbolicLink())throw Error('Invalid legacy fixture authority file');
  const raw=fs.readFileSync(file);fs.writeFileSync(archive,raw,{flag:'wx'});
  record.sha256=crypto.createHash('sha256').update(raw).digest('hex');record.archive=path.basename(archive);
  fs.unlinkSync(file);
 }
 const report=path.join(evidence,'r297-independent-preferences-fixtures.json');
 const rows=fs.existsSync(report)?JSON.parse(fs.readFileSync(report)):[];rows.push(record);fs.writeFileSync(report,JSON.stringify(rows,null,2)+'\n');return record;
}
module.exports={resetLegacyPreferences};
if(require.main===module){try{console.log(JSON.stringify(resetLegacyPreferences(process.argv[2],process.argv[3])))}catch(error){console.error(error.message);process.exitCode=1}}
