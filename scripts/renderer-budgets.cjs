'use strict';
function suiteBudgetFor(platform){return platform==='linux'?270000:240000;}
const suiteBudget=suiteBudgetFor(process.platform);
function fileBudget(platform){return platform==='linux'?100000:platform==='win32'?120000:90000;}
function withinBudgets(platform,files,duration){return files.every(row=>row.duration_ms<=fileBudget(platform))&&duration<=suiteBudgetFor(platform);}
module.exports={fileBudget,suiteBudget,withinBudgets};
