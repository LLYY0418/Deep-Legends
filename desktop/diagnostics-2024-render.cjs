// Run explicitly with Electron (not Node): tests actual Chromium layout and PNG pixels.
const {app, BrowserWindow} = require("electron");
const fs = require("node:fs");
const path = require("node:path");
const assert = require("node:assert/strict");
const root = path.resolve(__dirname,"..");
app.setPath("userData", "/private/tmp/deep-legends-2024-render-profile");
app.whenReady().then(async () => {
  const win = new BrowserWindow({show:false,width:1450,height:1050,webPreferences:{sandbox:true,contextIsolation:true}});
  const css = ["app.css","gameplay.css","champions.css"].map(file=>fs.readFileSync(path.join(root, "backend", "web",file),"utf8")).join("\n");
  const gameplay = fs.readFileSync(path.join(root,"backend/web/gameplay.js"),"utf8");
  const shell = gameplay.slice(gameplay.indexOf("  function matchTableShell("),gameplay.indexOf("  function renderMatchOverview("));
  const fixture = file => `data:image/png;base64,${fs.readFileSync(path.join(root,"testdata/diagnostics-2024",file)).toString("base64")}`;
  await win.loadURL("data:text/html;charset=utf-8,"+encodeURIComponent(`<html><head><style>${css}
    body{display:block;padding:24px;overflow:auto} #teams{display:grid;gap:12px} .icons{display:flex;gap:20px;margin:20px}.icons .augment-icon{width:100px;height:100px}
  </style></head><body><div id="teams"></div><div class="icons"><div class="is-gold"><span class="augment-icon"><img id="bonk" src="${fixture("bonk-large.png")}"></span></div><div class="is-prismatic"><span class="augment-icon"><img id="bear" src="${fixture("drop-bear.png")}"></span></div></div></body></html>`));
  await win.webContents.executeJavaScript(fs.readFileSync(path.join(root,"backend/web/augment-artwork.js"),"utf8"));
  const results = await win.webContents.executeJavaScript(`(async()=>{
    ${shell}
    const row=(name,items,score=true)=>'<tr><td><button class="participant-link"><span class="game-icon is-small"></span><span class="participant-name">'+name+'</span></button></td><td><div class="match-score-cell">'+(score?'9.2 MVP':'')+'</div></td><td>17 / 4 / 11<small>7.00:1</small></td><td>35,823<small>承伤 125,114</small></td><td>12 / 2</td><td>257<small>8.4/分钟</small></td><td><div class="table-items">'+Array.from({length:items},()=>'<span class="item-tooltip"><span class="game-icon is-small"></span></span>').join('')+'</div></td></tr>';
    document.querySelector('#teams').innerHTML= '<section class="team-overview is-blue">'+matchTableShell(row('梦短梦长俱是梦#11401',7)+row('The | snrt#67171',0,false))+'</section><section class="team-overview is-red">'+matchTableShell(row('短名',6)+row('极其漫长的玩家名称用于测试溢出'.repeat(8),7))+'</section>';
    const measurements=[];
    for(const width of [1360,1024,920,640]){
      document.querySelector('#teams').style.width=width+'px';
      const tables=[...document.querySelectorAll('.match-table')];
      const columns=tables.map(table=>[...table.querySelectorAll('th')].map(cell=>{const r=cell.getBoundingClientRect();return [r.x,r.width]}));
      const overflow=[...document.querySelectorAll('.table-items')].some(el=>el.scrollWidth>el.clientWidth+1);
      measurements.push({width,columns,overflow});
    }
    document.querySelector('#teams').style.width='1360px';
    const icons=[];
    for(const id of ['bonk','bear']){
      const image=document.getElementById(id);await image.decode();
      window.deepLegendsAugmentArtwork.prepare(image);
      const canvas=image.parentElement.querySelector('canvas');
      const pixels=canvas?.getContext('2d').getImageData(0,0,canvas.width,canvas.height).data;
      let gold=0,visible=0,bright=0;
      if(pixels)for(let i=0;i<pixels.length;i+=4){if(pixels[i+3]<160)continue;visible++;if(pixels[i]>pixels[i+1]&&pixels[i+1]>pixels[i+2])gold++;if(pixels[i]>225&&pixels[i+1]>205&&pixels[i+2]>170)bright++;}
      icons.push({id,tinted:!!canvas,goldRatio:visible?gold/visible:0,bright});
    }
    return {measurements,icons};
  })()`);
  try {
    for (const sample of results.measurements) {
      assert.equal(sample.overflow,false,`items overflow at ${sample.width}`);
      sample.columns[0].forEach((col,i)=>col.forEach((value,j)=>assert.ok(Math.abs(value-sample.columns[1][i][j])<.1,`column ${i} misaligned at ${sample.width}`)));
    }
    assert.equal(results.icons[0].tinted,true,"Bonk neutral glyph was not recognized");
    assert.ok(results.icons[0].goldRatio>.2 && results.icons[0].bright>500,"Bonk should have pale gold highlights, not a uniformly dark gold rim");
    assert.equal(results.icons[1].tinted,false,"colored Drop Bear artwork was overwritten");
    fs.writeFileSync("/private/tmp/deep-legends-2024-render.png",(await win.webContents.capturePage()).toPNG());
    console.log(JSON.stringify(results));
    app.exit(0);
  } catch(error) {console.error(error,JSON.stringify(results));app.exit(1);}
}).catch(error=>{console.error(error);app.exit(1);});
