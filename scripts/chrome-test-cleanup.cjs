'use strict';
// Test processes only. Cleanup never changes the guard's exit code.
const fs = require('node:fs');
const realClock = {setTimeout, clearTimeout};
const warning = message => console.error('cleanup warning: ' + String(message).replace(/[\r\n]+/g, ' '));
function hasExited(child) {
 return child.exitCode != null || child.signalCode != null;
}
function stopChild(child, {clock = realClock, exitTimeoutMs = 5000, warn = warning} = {}) {
 if (!child || hasExited(child)) return Promise.resolve(true);
 return new Promise(resolve => {
  let timer, finished = false, forced = false;
  const finish = stopped => {
   if (finished) return;
   finished = true; clock.clearTimeout(timer);
   child.off('exit', exited); child.off('close', exited); child.off('error', failed);
   resolve(stopped);
  };
  const exited = () => finish(true);
  // A spawn error has no process to wait for. A kill error is not proof of exit.
  const failed = error => {if (!child.pid) finish(true); else warn(error.message);};
  const deadline = () => {
   if (forced) {warn('child did not exit after SIGKILL; profile retained'); finish(false); return;}
   forced = true;
   try {child.kill('SIGKILL');} catch (error) {warn(error.message);}
   if (!finished) timer = clock.setTimeout(deadline, exitTimeoutMs);
  };
  child.once('exit', exited); child.once('close', exited); child.once('error', failed);
  timer = clock.setTimeout(deadline, exitTimeoutMs);
  try {child.kill('SIGTERM');} catch (error) {failed(error);}
 });
}
async function closeServer(server, {clock = realClock, serverTimeoutMs = 2000, warn = warning} = {}) {
 if (!server) return;
 await new Promise(resolve => {
  let finished = false;
  const done = error => {if (finished) return; finished = true; clock.clearTimeout(timer); if (error && error.code !== 'ERR_SERVER_NOT_RUNNING') warn(error.message); resolve();};
  const timer = clock.setTimeout(() => {warn('server close timed out'); done();}, serverTimeoutMs);
  try {server.close(done); server.closeAllConnections?.();} catch (error) {done(error);}
 });
}
async function removeDirectories(directories, {clock = realClock, retryDelayMs = 150, remove = directory => fs.rmSync(directory, {recursive:true, force:true}), warn = warning} = {}) {
 for (const directory of directories.filter(Boolean)) {
  for (let attempt = 0; attempt <= 8; attempt++) {
   try {await remove(directory); break;}
   catch (error) {
    if (attempt === 8) {warn(directory + ' left behind (' + error.message + ')'); break;}
    await new Promise(resolve => clock.setTimeout(resolve, retryDelayMs));
   }
  }
 }
}
function removeDirectoriesSync(directories, {warn = warning, remove = directory => fs.rmSync(directory, {recursive:true, force:true, maxRetries:8, retryDelay:150})} = {}) {
 // Electron owns its current process: its quit callback cannot await itself.
 for (const directory of directories.filter(Boolean)) {
  try {remove(directory);}
  catch (error) {warn(directory + ' left behind (' + error.message + ')');}
 }
}
async function cleanupChromeTest({children = [], sockets = [], servers = [], directories = []} = {}, options = {}) {
 const warn = options.warn || warning;
 try {
  for (const socket of sockets) {try {await socket?.close();} catch (error) {warn(error.message);}}
  const stopped = await Promise.all(children.map(child => stopChild(child, options)));
  // Crashpad/updater descendants can inherit Chrome's pipes after Chrome exits.
  // The guard has already recorded its result; release only these owned readers.
  children.forEach((child,index)=>{
   if (!child || !stopped[index]) return;
   for (const stream of [child.stdin,child.stdout,child.stderr]) {
    try {stream?.destroy?.();} catch (error) {warn(error.message);}
   }
  });
  await Promise.all(servers.map(server => closeServer(server, options)));
  if (stopped.every(Boolean)) await removeDirectories(directories, options);
 } catch (error) {warn(error.message);}
}
module.exports = {cleanupChromeTest, stopChild, closeServer, removeDirectories, removeDirectoriesSync};
