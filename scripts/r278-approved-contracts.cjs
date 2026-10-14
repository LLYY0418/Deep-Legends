'use strict';
const assert=require('node:assert/strict');
function assertOfficialOutage({text,styleCount,retryText}) {
 assert(!/\d+\s*秒/.test(text),'official outage must not display countdown seconds');
 assert.equal(styleCount,1,'official outage uses one status style');
 assert(retryText.includes('立即重试'),'official outage retains immediate retry');
}
module.exports={assertOfficialOutage};
