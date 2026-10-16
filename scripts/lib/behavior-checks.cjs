"use strict";
const {query,identifiersIn}=require("../contract-impact.cjs"),{analyze}=require("../fixture-alignment-check.cjs");
function analyzeChanges(source,changes) {
  const product=changes.filter(row=>!/(?:\.test\.[cm]js|_test\.go)$/.test(row.file)&&/^(?:backend\/web\/.*\.(?:js|css|html)|backend\/[^/]+\.go|desktop\/(?:main|preload)\.cjs)$/.test(row.file));
  const identifiers=[...new Set(product.flatMap(row=>identifiersIn(row.delta)))];
  const alignmentTriggered=identifiers.some(token=>token.startsWith("/api/"))||product.some(row=>/announcements|\bpopup\b|showModal/.test(row.delta));
  return {changedProductFiles:product.map(row=>row.file),conservativeFiles:product.filter(row=>row.basis==='whole-file-conservative').map(row=>row.file),contractImpact:query(source,identifiers),fixtureAlignment:alignmentTriggered?analyze(source):null};
}
module.exports={analyzeChanges};
