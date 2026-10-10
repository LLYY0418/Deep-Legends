'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),os=require('node:os'),crypto=require('node:crypto'),{createRequire}=require('node:module');
const root=path.resolve(__dirname,'..'),original=path.join(root,'desktop/verify-embedded-riot-key.cjs');
let policy=require(original);
if(process.env.R266_KEY_POLICY_SOURCE){const module={exports:{}};new Function('require','module','__dirname',fs.readFileSync(process.env.R266_KEY_POLICY_SOURCE,'utf8'))(createRequire(original),module,path.dirname(original));policy=module.exports;}
function cipher(text){const key=crypto.createHash('sha256').update(['deep','legends','hexcore','loot','kr-riot-channel','v1'].join('\x1f')).digest(),nonce=crypto.randomBytes(12),c=crypto.createCipheriv('aes-256-gcm',key,nonce);return Buffer.concat([nonce,c.update(text),c.final(),c.getAuthTag()]).toString('base64');}
test('R266 public artifacts require ciphertext, reject plaintext and verify the exact fake key',t=>{
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'r266-key-policy-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));const file=path.join(dir,'fake-backend');
 fs.writeFileSync(file,'no cipher');assert.throws(()=>policy.verifyRiotKeyPolicy(file,'public'),/ciphertext/);
 const fake=cipher('RGAPI-00000000-0000-0000-0000-000000000000');fs.writeFileSync(file,'prefix '+fake+' suffix');assert(policy.verifyRiotKeyPolicy(file,'public',true));assert(policy.verifyRiotKeyPolicy(file,'private'));
 fs.writeFileSync(file,'RGAPI- '+fake);assert.throws(()=>policy.verifyRiotKeyPolicy(file,'public'),/plaintext/);
 fs.writeFileSync(file,cipher('RGAPI-11111111-1111-1111-1111-111111111111'));assert.throws(()=>policy.verifyRiotKeyPolicy(file,'public',true),/expected fake/);
 for(const name of ['Setup.exe','latest.json','SHA256SUMS-public.txt']){assert(policy.verifyNoPlaintext(Buffer.from(name)));assert.throws(()=>policy.verifyNoPlaintext(Buffer.from(name+' RGAPI-')),/plaintext/);}
});
test('R266 formal release rejects absent secret before encryption; CI has only the fake key',()=>{
 const release=fs.readFileSync(path.join(root,'.github/workflows/release.yml'),'utf8'),ci=fs.readFileSync(path.join(root,'.github/workflows/ci.yml'),'utf8');
 assert.match(release,/RIOT_API_KEY: \$\{\{ secrets\.RIOT_API_KEY \}\}/);assert.match(release,/if \(\[string\]::IsNullOrWhiteSpace\(\$env:RIOT_API_KEY\)\) \{ throw/);
 const guard=release.indexOf('if ([string]::IsNullOrWhiteSpace($env:RIOT_API_KEY))'),mask=release.indexOf('::add-mask::$env:RIOT_API_KEY'),encrypt=release.indexOf('-encrypt-riot-key env:RIOT_API_KEY');assert(guard<mask && mask<encrypt);
 assert.doesNotMatch(ci,/secrets\.RIOT_API_KEY/);assert.match(ci,/RGAPI-00000000-0000-0000-0000-000000000000/);assert.match(ci,/--expect-fake/);
});
