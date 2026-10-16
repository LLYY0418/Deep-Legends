'use strict';
const child=require('node:child_process').spawnSync('gofmt',['-l',...process.argv.slice(2)],{encoding:'utf8'});
if(child.status!==0||child.error||child.stdout.trim()){console.error(child.stdout||child.stderr||child.error);process.exitCode=1;}
