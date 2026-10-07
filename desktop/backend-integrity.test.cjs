"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),os=require("node:os"),path=require("node:path");
const {backendHash,verifyBackend}=require("./backend-integrity.cjs");
const {generateBackendDigest}=require("./generate-backend-digest.cjs");
test("R232 generated digest detects replaced backend without adjacent checksum trust",t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),"r232-digest-"));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  fs.mkdirSync(path.join(root,"backend"));const file=path.join(root,"backend/loot-service.exe");fs.writeFileSync(file,Buffer.alloc(3*1024*1024,42));
  const digest=generateBackendDigest(root);assert.equal(digest,backendHash(file));assert.equal(require(path.join(root,"backend-digest.cjs")),digest);assert.equal(verifyBackend(file,digest),true);
  fs.writeFileSync(file,"replaced");fs.writeFileSync(file+".sha256",backendHash(file));assert.throws(()=>verifyBackend(file,digest),/校验失败/);assert.throws(()=>verifyBackend(file,""),/校验失败/);
});
