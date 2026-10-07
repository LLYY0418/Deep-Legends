const {probe}=require('../../../../scripts/r243-electron.cjs');
probe({phase:'after',state:'NETWORK_LOCKED',recovery:true,index:8,savedBounds:{x:90,y:60,width:1200,height:780,maximized:false},activation:true}).then(result=>console.log(JSON.stringify({activation:result.activationBusy,evidence:result.cacheEvidence,restored:result.restored}))).catch(error=>{console.error(error);process.exitCode=1});
