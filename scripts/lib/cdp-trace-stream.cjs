'use strict';
const READ_BYTES=256*1024;
async function readTraceStream(send,handle,{keep}) {
 const chunks=[];let readFailed=false;
 try {
  if(keep)for(;;){const part=await send('IO.read',{handle,size:READ_BYTES});chunks.push(Buffer.from(part.data,part.base64Encoded?'base64':'utf8'));if(part.eof)break;}
  return Buffer.concat(chunks);
 }catch(error){readFailed=true;throw error;}
 finally{try{await send('IO.close',{handle});}catch(error){if(!readFailed)throw error;}}
}
module.exports={readTraceStream,READ_BYTES};
