'use strict';
const assert=require('node:assert/strict');
const mutationJobs=['quality-mutations','r277-proof-linux-1','r277-proof-linux-2','r277-browser-mutations','mutations-1','mutations-3'];
function verifyMutationRouting(ci){for(const name of mutationJobs){const job=ci.jobs[name];assert(job,`${name} missing`);assert.equal(job.if,undefined,`${name} must run on every push without a branch selector`);assert.equal(job.needs,undefined,`${name} must not depend on another test job`);assert.equal(job['continue-on-error'],undefined);}}
module.exports={mutationJobs,verifyMutationRouting};
