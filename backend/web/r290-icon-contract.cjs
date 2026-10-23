"use strict";
const assert=require("node:assert/strict");
function assertArrow(svg){
 assert(svg,"download glyph exists");
 assert.equal(svg.getAttribute("viewBox"),"0 0 24 24");
 const path=svg.querySelector("path");assert(path,"single linear arrow and tray");
 const d=path.getAttribute("d");assert.match(d,/M12 3v12/);assert.match(d,/m-5-5 5 5 5-5/);assert.match(d,/M4 16v5h16v-5/);
 assert.equal(svg.querySelector(".hex-track,.hex-progress"),null);
 const progress=svg.parentElement.querySelector(".update-border .update-border-progress");assert(progress,"progress stays on button border");
 assert.equal(progress.getAttribute("pathLength"),"100");assert.equal(progress.getAttribute("stroke-dashoffset"),"100");
 assert.equal(progress.tagName.toLowerCase(),"rect");
}
module.exports={assertArrow};
