'use strict';
// Normal samples discard their trace. Read retained evidence in bounded chunks.
async function readTraceStream(send,handle,keep){
 let trace='',failure;
 try {
  if(keep)for(;;){const part=await send('IO.read',{handle,size:1024*1024});trace+=part.base64Encoded?Buffer.from(part.data,'base64').toString():part.data;if(part.eof)break;}
  return trace;
 } catch(error){failure=error;throw error;}
 finally {try{await send('IO.close',{handle});}catch(error){if(!failure)throw error;}}
}
module.exports={readTraceStream};
