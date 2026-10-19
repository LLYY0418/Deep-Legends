'use strict';
const assert=require('node:assert/strict');
function assertArrow(svg){
 const shaft=svg.querySelector('.arrow').getAttribute('d').match(/^M[\d.]+ ([\d.]+)V([\d.]+)$/);
 assert(shaft,'vertical arrow shaft');
 const height=Math.abs(Number(shaft[1])-Number(shaft[2]));
 const track=svg.querySelector('.hex-track').getAttribute('d').match(/^M[\d.]+ ([\d.]+).*L[\d.]+ ([\d.]+)/);
 assert(track,'hexagon vertical endpoints');
 const hexHeight=Math.abs(Number(track[2])-Number(track[1]));
 assert(height/hexHeight>=.55,`arrow height ratio ${height/hexHeight}`);
 assert.equal(svg.querySelector('.hex-progress').getAttribute('pathLength'),'59');
}
module.exports={assertArrow};
