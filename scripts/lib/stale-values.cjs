'use strict';
const cp=require('node:child_process'),fs=require('node:fs'),path=require('node:path');
function literals(source){
 const values=new Set();
 // Strings shorter than three characters and single-digit numbers are noise.
 for(const m of source.matchAll(/(["'`])((?:\\.|(?!\1)[^\\])*?)\1/g)){const value=m[2];if(value.length>=3&&!value.includes('\n'))values.add(value);}
 const code=source.replace(/(["'`])((?:\\.|(?!\1)[^\\])*?)\1/g,'');
 for(const m of code.matchAll(/\b\d+(?:\.\d+){0,2}\b/g))if(m[0].length>1)values.add(m[0]);
 return values;
}
function product(file){if(file.startsWith('desktop/')&&!['desktop/package.json','desktop/package-lock.json',...require('../../desktop/package.json').build.files.map(f=>'desktop/'+f)].includes(file))return false;return /^(?:backend\/|desktop\/|installer\/|relay\/riot-worker\/)/.test(file)&&!/(?:_test\.go|\.test\.[cm]?js|-harness\.cjs|-layout\.cjs|-browser\.cjs)$|\/(?:testdata|test-fixtures|node_modules)\//.test(file);}
function testInput(file){return /^(?:scripts\/|backend\/|desktop\/|installer\/|relay\/)/.test(file)&&(/(?:_test\.go|\.test\.[cm]?js|\.(?:py|ps1))$|\/(?:testdata|test-fixtures)\/|^scripts\//.test(file));}
function scanValues(changes,files){
 const old=[];
 for(const change of changes){if(!product(change.file))continue;const removed=literals(change.removed),added=literals(change.added);for(const value of removed)if(!added.has(value))old.push({value,from:change.file});}
 const hits=[];
 for(const [file,source]of Object.entries(files)){if(!testInput(file))continue;for(const [index,line]of source.split('\n').entries())for(const row of old){const at=line.indexOf(row.value);if(at<0)continue;if(/^\d/.test(row.value)&&(/[\d.]/.test(line[at-1]||' ')||/[\d.]/.test(line[at+row.value.length]||' ')))continue;hits.push({file,line:index+1,...row,source:line.trim()});}}
 return hits;
}
function scanRepository(root,base){
 const git=args=>cp.execFileSync('git',args,{cwd:root,encoding:'utf8',maxBuffer:64*1024*1024});
 const changes=[],diff=git(['diff','--no-ext-diff','--no-renames','--unified=0',base,'--']);let current;
 for(const line of diff.split('\n')){if(line.startsWith('+++ b/')){current={file:line.slice(6),removed:'',added:''};changes.push(current);}else if(current&&line[0]==='-'&&!line.startsWith('---'))current.removed+=line.slice(1)+'\n';else if(current&&line[0]==='+'&&!line.startsWith('+++'))current.added+=line.slice(1)+'\n';}
 const files={};for(const file of git(['ls-files','-co','--exclude-standard','-z']).split('\0').filter(Boolean))if(testInput(file)&&fs.existsSync(path.join(root,file))&&/\.(?:go|cjs|mjs|js|json|py|ps1|html|txt)$/.test(file))files[file]=fs.readFileSync(path.join(root,file),'utf8');
 return scanValues(changes,files);
}
module.exports={literals,product,testInput,scanValues,scanRepository};
