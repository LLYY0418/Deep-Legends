'use strict';
const assert=require('node:assert/strict');
const {diagnostic}=require('../release-validation-policy.json');
function fullTriggers(legacy){
 const expected=structuredClone(legacy);assert.deepEqual(expected.push.branches,['**'],'original full branch coverage must remain the baseline');
 expected.push.branches.push('!'+diagnostic.branchPrefix+'*');return expected;
}
function isDiagnosticBranch(ref){return ref.startsWith('refs/heads/'+diagnostic.branchPrefix);}
function branchRuns(push,ref){
 if(!ref.startsWith('refs/heads/')||!push.branches)return false;
 const branch=ref.slice('refs/heads/'.length);let selected=false;
 for(const pattern of push.branches){const negative=pattern.startsWith('!'),text=negative?pattern.slice(1):pattern;const regex=new RegExp('^'+text.replace(/[.+?^${}()|[\]\\]/g,'\\$&').replace(/\*/g,'.*')+'$');if(regex.test(branch))selected=!negative;}
 return selected;
}
module.exports={fullTriggers,isDiagnosticBranch,branchRuns};
