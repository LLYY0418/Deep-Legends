"use strict";
// Read-only checks of the single default public Windows audit package.
const fs=require("node:fs"),path=require("node:path"),crypto=require("node:crypto"),assert=require("node:assert/strict");
const root=path.resolve(__dirname,".."),desktop=path.join(root,"desktop"),output=path.join(root,"dist/R248-no-license-public");
const digest=file=>crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
(async()=>{
 const receipt=JSON.parse(fs.readFileSync(path.join(output,"release-build.json"))),dir=path.join(output,"win-unpacked"),archive=path.join(dir,"resources/app.asar"),backend=path.join(dir,"resources/app.asar.unpacked/backend/loot-service.exe"),exe=path.join(dir,"Deep Legends.exe");
 const asar=require(path.join(desktop,"node_modules/@electron/asar")),entries=asar.listPackage(archive),pkg=JSON.parse(asar.extractFile(archive,"package.json")),flag=asar.extractFile(archive,"license-build.cjs").toString();
 assert.equal(receipt.version,"0.12.75");assert.equal(pkg.version,"0.12.75");assert.equal(receipt.mode,"public");assert.ok(flag.includes("enabled: false"));
 const setupName=Object.keys(receipt.assets)[0],setup=path.join(output,setupName);assert.match(setupName,/-public\.exe$/);assert.equal(digest(setup),receipt.assets[setupName]);assert.equal(digest(backend),receipt.backendSHA256);
 require(path.join(desktop,"verify-license-release.cjs")).verifyLicenseRelease(backend);
 require(path.join(desktop,"verify-embedded-riot-key.cjs")).verifyRiotKeyPolicy(backend,"public");
 require(path.join(desktop,"verify-packaged-runtime.cjs")).verifyPackagedRuntime(archive);
 assert.ok(asar.extractFile(archive,"backend-digest.cjs").toString().includes(digest(backend)));
 assert.ok(!entries.some(entry=>/testdata|\.test\.|\.license\.|_test\.go|protocol-vectors|verify-license|build-license/.test(entry)),"fixtures entered runtime archive");
 const forbidden=["license.yinxiaobia.net","license-staging.yinxiaobia.net","staging-2026-10","E8GWwjDvAE_6_6rx5CTT0XqjKrKssR3U3D2z49cGagE","服务端保留授权事件记录（时间、设备摘要前8位、版本号、结果），180 天后自动清理；授权记录与防重放计数器持续保留，用于授权校验。"];
 const vectors=JSON.parse(fs.readFileSync(path.join(root,"backend/testdata/license-protocol-vectors.json")));for(const [kid,key] of Object.entries(vectors.public_keys))forbidden.push(kid,key);
 const markers=forbidden.flatMap(value=>[Buffer.from(value),...(/^[A-Za-z0-9_-]{43}$/.test(value)?[Buffer.from(value,"base64url")]:[])]);
 for(const [name,bytes] of [["backend",fs.readFileSync(backend)],["setup",fs.readFileSync(setup)],...entries.filter(e=>e.endsWith(".cjs")).map(e=>[e,asar.extractFile(archive,e.replace(/^\//,""))])])for(const marker of markers)assert.ok(!bytes.includes(marker),name+" contains authorization origin/kid/public key/privacy");
 const {getCurrentFuseWire,FuseV1Options}=require(path.join(desktop,"node_modules/@electron/fuses")),{FuseState}=require(path.join(desktop,"node_modules/@electron/fuses/dist/constants.js")),wire=await getCurrentFuseWire(exe),fuses={};
 for(const [name,enabled] of [["RunAsNode",false],["EnableNodeOptionsEnvironmentVariable",false],["EnableNodeCliInspectArguments",false],["EnableEmbeddedAsarIntegrityValidation",true],["OnlyLoadAppFromAsar",true]]){assert.equal(wire[FuseV1Options[name]],enabled?FuseState.ENABLE:FuseState.DISABLE);fuses[name]=enabled}
 const {NtExecutable,NtExecutableResource}=await import(path.join(desktop,"node_modules/resedit/dist/index.js")),res=NtExecutableResource.from(NtExecutable.from(fs.readFileSync(exe))),integrity=res.entries.find(e=>e.type==="INTEGRITY"&&e.id==="ELECTRONASAR");assert.ok(integrity);
 const records=JSON.parse(Buffer.from(integrity.bin).toString()),{header}=await require(path.join(desktop,"node_modules/app-builder-lib/out/asar/asar.js")).readAsarHeader(archive),headerSHA=crypto.createHash("sha256").update(header).digest("hex");assert.ok(records.some(r=>r.file==="resources\\app.asar"&&r.alg==="SHA256"&&r.value===headerSHA));
 const result={...receipt,setup,setup_sha256:digest(setup),backend_sha256:digest(backend),archive_sha256:digest(archive),audit_only:true,license_enabled:false,authorization_markers_absent:true,public_riot_key_policy:true,fixed_backend_digest:true,fuses,asar_integrity:records,update_validation:"0.12.76 unsigned manifest validation + size/SHA256; actual published asset passed local test",windows_execution:"未在 Windows 实跑"};
 fs.writeFileSync(path.join(root,"docs/history/reports/r248/package-audit.json"),JSON.stringify(result,null,2)+"\n");fs.writeFileSync(path.join(output,"r248-audit.json"),JSON.stringify(result,null,2)+"\n");console.log(JSON.stringify(result));
})().catch(e=>{console.error(e);process.exitCode=1});
