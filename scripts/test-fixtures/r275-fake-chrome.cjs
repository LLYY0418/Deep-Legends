'use strict';
const fs=require('node:fs');
const [mode,counter]=process.argv.slice(2);
const attempt=fs.existsSync(counter)?Number(fs.readFileSync(counter,'utf8'))+1:1;
fs.writeFileSync(counter,String(attempt));
if(mode==='exit'){process.stderr.write('fake early exit\n');process.exitCode=7;}
else{
 process.stdin.resume();const keepAlive=setInterval(()=>{},60000);
 process.on('SIGTERM',()=>{clearInterval(keepAlive);process.exit(0);});
 if(mode==='recover'&&attempt===2)process.stderr.write('fake second connected\nDevTools listening on ws://127.0.0.1:1234/devtools/browser/fake\n');
 else process.stderr.write(`fake held attempt ${attempt}\n`);
}
