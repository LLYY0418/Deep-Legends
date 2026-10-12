'use strict';
const suiteBudget=240000;
function fileBudget(platform){return platform==='win32'?120000:90000;}
function withinBudgets(platform,files,duration){return files.every(row=>row.duration_ms<=fileBudget(platform))&&duration<=suiteBudget;}
module.exports={fileBudget,suiteBudget,withinBudgets};
