'use strict';
// The captured r308 catalog intentionally has one shard per row. Geometry
// proof/screenshot fixtures use the production default's complete three choices.
function completeRuneCatalog(input){
 const perk=(id,name,icon)=>({id,name,iconPath:'ddragon:/cdn/img/perk-images/StatMods/'+icon});
 const adaptive=perk(5008,'适应之力','StatModsAdaptiveForceIcon.png'),health=perk(5001,'成长生命值','StatModsHealthPlusIcon.png');
 return {...(input.catalog||input),statModSlots:[
  {type:'kStatMod',perks:[perk(5005,'攻击速度','StatModsAttackSpeedIcon.png'),adaptive,perk(5007,'技能急速','StatModsCDRScalingIcon.png')]},
  {type:'kStatMod',perks:[adaptive,perk(5010,'移动速度','StatModsMovementSpeedIcon.png'),health]},
  {type:'kStatMod',perks:[perk(5011,'生命值','StatModsHealthScalingIcon.png'),perk(5013,'韧性','StatModsTenacityIcon.png'),health]},
 ]};
}
module.exports={completeRuneCatalog};
