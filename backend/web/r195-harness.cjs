'use strict';
const {fixture}=require('./r190-harness.cjs');
function effectFixture(source) {
 const f=fixture(source);
 f.perks.push({id:8008,name:'致命节奏',shortDesc:'攻击一个敌方英雄会为你提供攻击速度。',eogDescs:['最大攻速运转时间：@eogvar1@:@eogvar2@<br>已造成的伤害：@eogvar2@']},{id:8304,name:'神奇之鞋',shortDesc:'在12分钟时获得免费的鞋子，参与击杀会让鞋子提前到达。',eogDescs:['鞋子到达时间：@eogvar1@:@eogvar2@@eogvar3@']});
 f.subject={...f.subject,perkIds:[8008,9111,9103,8017,8304,8347],statModIds:[5005,5008,5001],perkStats:[...f.subject.perkStats,{perkId:8008,vars:[932,972,0]},{perkId:8304,vars:[7,3,0]}]};
 return f;
}
module.exports={effectFixture};
