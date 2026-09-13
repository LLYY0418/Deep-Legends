"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),os=require("node:os"),path=require("node:path"),crypto=require("node:crypto");
const {makeRelease,latestNotes,releaseName}=require("./make-release.cjs");
const {recordReleaseBuild}=require("../desktop/release-build.cjs");
test("setup-only release hashes actual renamed bytes and emits exactly three files",()=>{
 const root=fs.mkdtempSync(path.join(os.tmpdir(),"deep-legends-release-test-"));
 try {
  fs.mkdirSync(path.join(root,"desktop"));fs.mkdirSync(path.join(root,"dist","desktop"),{recursive:true});
  fs.writeFileSync(path.join(root,"desktop","package.json"),JSON.stringify({version:"0.12.0"}));
  fs.writeFileSync(path.join(root,"CHANGELOG.md"),'# 更新日志\n\n## 0.12.0 — 2026-09-11\n\n### 新增\n- 更新\n\n## 0.11.2 — 2026-09-10\n- 旧内容\n');
  const fingerprint="a1b2c3d4e5f6",bytes=Buffer.from("actual installer bytes");
  fs.writeFileSync(path.join(root,"dist","desktop",`Deep Legends Setup 0.12.0.exe`),bytes);
  const backend=path.join(root,"desktop","backend","loot-service.exe");
  const packaged=path.join(root,"dist","desktop","win-unpacked","resources","app.asar.unpacked","backend","loot-service.exe");
  for(const file of [backend,packaged]){fs.mkdirSync(path.dirname(file),{recursive:true});fs.writeFileSync(file,`MZ-test-backend-${fingerprint}`);}
  assert.throws(()=>makeRelease({root,fingerprint}),/缺少构建验证记录/);
  recordReleaseBuild({root,fingerprint,mode:"public"});
  const manifest=makeRelease({root,fingerprint,publishedAt:"2026-09-11T12:00:00Z"});
  const directory=path.join(root,"dist","release"),names=fs.readdirSync(directory);
  assert.deepEqual(names.sort(),["Deep-Legends-Setup-0.12.0.exe","SHA256SUMS.txt","latest.json"]);
  assert.equal(manifest.asset.size,bytes.length);assert.equal(manifest.asset.sha256,crypto.createHash("sha256").update(bytes).digest("hex"));
  assert.deepEqual(fs.readFileSync(path.join(directory,manifest.asset.name)),bytes);
  assert.equal(manifest.asset.url,`https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.0/${manifest.asset.name}`);
  assert.equal(manifest.notes,"### 新增\n- 更新");assert.deepEqual(JSON.parse(fs.readFileSync(path.join(directory,"latest.json"))),manifest);
  for(const line of fs.readFileSync(path.join(directory,"SHA256SUMS.txt"),"utf8").trim().split("\n")){
   const [hash,name]=line.split("  ");assert.equal(hash,crypto.createHash("sha256").update(fs.readFileSync(path.join(directory,name))).digest("hex"));
  }
  fs.writeFileSync(path.join(directory,"stale.exe"),"old");makeRelease({root,fingerprint});assert.equal(fs.readdirSync(directory).length,3);
  fs.appendFileSync(path.join(root,"dist","desktop",`Deep Legends Setup 0.12.0.exe`),"changed");
  assert.throws(()=>makeRelease({root,fingerprint}),/发布文件与已验证构建不一致/);
  fs.writeFileSync(path.join(root,"dist","desktop",`Deep Legends Setup 0.12.0.exe`),bytes);
  fs.appendFileSync(backend,"changed");assert.throws(()=>makeRelease({root,fingerprint}),/Backend changed/);
  fs.writeFileSync(backend,`MZ-test-backend-${fingerprint}`);
  const receiptPath=path.join(root,"dist","desktop","release-build.json"),receipt=JSON.parse(fs.readFileSync(receiptPath));
  receipt.mode="private";fs.writeFileSync(receiptPath,JSON.stringify(receipt));
  assert.throws(()=>makeRelease({root,fingerprint}),/禁止发布/);
  assert.equal(fs.readdirSync(directory).length,3,"failed preflight must preserve previous complete release");
 }finally{fs.rmSync(root,{recursive:true,force:true});}
});
test("invalid release identities and spaced assets are refused",()=>{
 assert.throws(()=>releaseName("Deep Legends.exe"));assert.throws(()=>releaseName("../evil.exe"));
 assert.throws(()=>latestNotes("## 0.11.2 — 2026-09-11\n- old","0.12.0"));
 assert.throws(()=>latestNotes("## 0.12.0 — 2026-09-11\n\n","0.12.0"));
});
