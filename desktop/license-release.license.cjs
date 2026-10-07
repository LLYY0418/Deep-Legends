"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),os=require("node:os"),path=require("node:path");
const {verifyLicenseRelease}=require("./verify-license-release.cjs");
test("R233 release verification rejects staging origins and test trust markers",t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),"r233-release-"));t.after(()=>fs.rmSync(root,{recursive:true,force:true}));
  const file=path.join(root,"backend-public.exe");fs.writeFileSync(file,"MZ production backend https://license.yinxiaobia.net");assert.equal(verifyLicenseRelease(file,root,true),true);
  for(const marker of ["https://license-staging.yinxiaobia.net","https://manage-staging.yinxiaobia.net","test-issuer","test-update"]){fs.writeFileSync(file,"MZ "+marker);assert.throws(()=>verifyLicenseRelease(file,root,true),/staging|test trust/)}
  fs.mkdirSync(path.join(root,"backend/testdata"),{recursive:true});fs.writeFileSync(path.join(root,"backend/testdata/license-protocol-vectors.json"),JSON.stringify({public_keys:{"vector-test-id":"vector-public-marker"}}));fs.writeFileSync(file,"MZ vector-public-marker");assert.throws(()=>verifyLicenseRelease(file,root,true),/test trust/);
});
test("R233 staging is a separately named manual build, outside release workflow",()=>{
  const {stagingOutput}=require("../scripts/build-license-staging.cjs");
  assert.throws(()=>stagingOutput(["--output",path.join(os.tmpdir(),"ordinary-release")]),/STAGING/);
  assert.equal(require("./app-title.cjs"),"Deep Legends");
  const root=path.resolve(__dirname,"..");const release=fs.readFileSync(path.join(root,".github/workflows/release.yml"),"utf8");
  assert.doesNotMatch(release,/build-license-staging/);
  assert.match(fs.readFileSync(path.join(root,"build-desktop.sh"),"utf8"),/go build -tags=/);
  assert.match(fs.readFileSync(path.join(root,"build-desktop-windows.ps1"),"utf8"),/go build -tags=/);
  assert.match(fs.readFileSync(path.join(__dirname,"main.cjs"),"utf8"),/page-title-updated.*preventDefault/);
});
test("R234 final vector kid, public key and device seed cannot enter release binary",t=>{
  const root=path.resolve(__dirname,".."),vectors=JSON.parse(fs.readFileSync(path.join(root,"backend/testdata/license-protocol-vectors.json"),"utf8"));
  const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r234-vector-exclusion-"));t.after(()=>fs.rmSync(temp,{recursive:true,force:true}));
  const file=path.join(temp,"backend-public.exe");
  const markers=[];
  for(const [kid,pub] of Object.entries(vectors.public_keys))markers.push(Buffer.from(kid),Buffer.from(pub),Buffer.from(pub,"base64url"));
  markers.push(Buffer.from(vectors.device_seed_hex),Buffer.from(vectors.device_seed_hex,"hex"));
  for(const marker of markers){assert.ok(marker.length);fs.writeFileSync(file,Buffer.concat([Buffer.from("MZ fixture "),marker]));assert.throws(()=>verifyLicenseRelease(file,root,true),/test trust/);}
});
test("R236 staging handoff public key is rejected by release packaging",t=>{
  const root=path.resolve(__dirname,".."),temp=fs.mkdtempSync(path.join(os.tmpdir(),"r236-release-"));
  t.after(()=>fs.rmSync(temp,{recursive:true,force:true}));
  const file=path.join(temp,"backend-public.exe"),key="E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE";
  for(const marker of [Buffer.from(key),Buffer.from(key,"base64url")]) {
    fs.writeFileSync(file,Buffer.concat([Buffer.from("MZ release "),marker]));
    assert.throws(()=>verifyLicenseRelease(file,root,true),/test trust/);
  }
});
test("R236 staging packaging requires the handed-off key and excludes vector material",t=>{
  const {verifyStagingBackend}=require("./verify-license-staging-pack.cjs");
  const temp=fs.mkdtempSync(path.join(os.tmpdir(),"r236-staging-"));t.after(()=>fs.rmSync(temp,{recursive:true,force:true}));
  const file=path.join(temp,"backend-staging-public.exe"),key="E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE";
  const valid=Buffer.from("MZ https://license-staging.yinxiaobia.net staging-2026-10 "+key);
  fs.writeFileSync(file,valid);assert.equal(verifyStagingBackend(file),true);
  fs.writeFileSync(file,valid.toString().replace(key,"F"+key.slice(1)));assert.throws(()=>verifyStagingBackend(file),/missing/);
  const vectors=JSON.parse(fs.readFileSync(path.join(__dirname,"../backend/testdata/license-protocol-vectors.json"),"utf8"));
  const forbidden=[Buffer.from("test-license-r233"),Buffer.from(vectors.device_seed_hex,"hex"),Buffer.from(Object.values(vectors.public_keys)[0],"base64url")];
  for(const marker of forbidden){fs.writeFileSync(file,Buffer.concat([valid,marker]));assert.throws(()=>verifyStagingBackend(file),/private\/test/);}
});
