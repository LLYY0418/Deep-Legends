'use strict';
// Chromium screenshots use non-interlaced 8-bit RGB/RGBA PNGs.
const assert=require('node:assert/strict'),zlib=require('node:zlib');
function decodePNG(buffer){
 assert.equal(buffer.subarray(0,8).toString('hex'),'89504e470d0a1a0a');
 let width,height,channels;const chunks=[];
 for(let offset=8;offset<buffer.length;){
  const length=buffer.readUInt32BE(offset),type=buffer.toString('ascii',offset+4,offset+8),data=buffer.subarray(offset+8,offset+8+length);
  if(type==='IHDR'){width=data.readUInt32BE(0);height=data.readUInt32BE(4);assert.equal(data[8],8);assert([2,6].includes(data[9]));channels=data[9]===2?3:4;assert.equal(data[12],0);}
  if(type==='IDAT')chunks.push(data);offset+=length+12;
 }
 const packed=zlib.inflateSync(Buffer.concat(chunks)),stride=width*channels,pixels=Buffer.alloc(height*stride);
 assert.equal(packed.length,(stride+1)*height);
 const paeth=(a,b,c)=>{const p=a+b-c,pa=Math.abs(p-a),pb=Math.abs(p-b),pc=Math.abs(p-c);return pa<=pb && pa<=pc?a:pb<=pc?b:c;};
 for(let y=0;y<height;y++){
  const filter=packed[y*(stride+1)];assert(filter<=4);
  for(let x=0;x<stride;x++){
   const i=y*stride+x,a=x>=channels?pixels[i-channels]:0,b=y?pixels[i-stride]:0,c=y && x>=channels?pixels[i-stride-channels]:0;
   const predictor=[0,a,b,Math.floor((a+b)/2),paeth(a,b,c)][filter];pixels[i]=(packed[y*(stride+1)+1+x]+predictor)&255;
  }
 }
 return {width,height,brightness(x,y){assert(x>=0 && y>=0 && x<width && y<height);const i=(Math.floor(y)*width+Math.floor(x))*channels;return .2126*pixels[i]+.7152*pixels[i+1]+.0722*pixels[i+2];}};
}
function regionBrightness(png,rect){
 const values=[];
 for(let y=Math.ceil(rect.top);y<Math.floor(rect.bottom);y++)for(let x=Math.ceil(rect.left);x<Math.floor(rect.right);x++)values.push(png.brightness(x,y));
 assert(values.length>0,'pixel region must be visible');return {min:Math.min(...values),max:Math.max(...values),mean:values.reduce((a,b)=>a+b,0)/values.length,pixels:values.length};
}
module.exports={decodePNG,regionBrightness};
