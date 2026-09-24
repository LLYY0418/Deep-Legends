# Deep Legends

Deep Legends 是一款 Windows 本地英雄联盟助手，把英雄数据、赛前准备、实时对局、召唤师战绩和客户端收藏整理在同一个桌面窗口。游戏客户端尚未启动时，可以先浏览英雄梯度与推荐；登录国服客户端后，还能查看本机账号相关内容并使用需要客户端配合的工具。

下面六张均为 **3840 × 2160 原始截图**，使用演示数据展示界面与布局。点击图片可打开完整分辨率原图。

## 界面一览

### 总览 · 从战绩回到每一场对局

总览将排位、近期表现和最近战绩放在一起。可以按模式筛选对局，查看英雄、K/D/A、装备、参团与补刀等信息；展开单场后还能继续看队伍分析、符文、出装路线和技能加点。搜索栏可打开其他召唤师的独立页签。

[![Deep Legends 总览界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/01-overview.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/01-overview.png)

### 英雄 · 按模式、位置和段位寻找思路

英雄页汇总韩服单/双排的梯度、胜率、选用率和禁用率，支持按位置、段位和英雄名称查找。切换到海克斯大乱斗或斗魂竞技场，可以查看各模式对应的英雄和构建推荐；点开英雄可继续研究符文、技能与出装。

[![Deep Legends 英雄界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/02-champions.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/02-champions.png)

### 对局 · 把赛前信息放在需要它的位置

进入英雄选择后，对局页结合当前阵容与位置展示符文、出装和技能建议。不同推荐来源可切换比较；确认方案后，可由玩家主动点击应用所选符文。页面也提供当前对局信息与英雄选择阶段的配置入口。

[![Deep Legends 对局界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/03-live.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/03-live.png)

### 收藏 · 一个地方查看账号物品

连接国服客户端后，可按皮肤与炫彩、账户与物品、三合一奖池、头像与旗帜浏览本机账号内容。材料、宝箱、英雄碎片和待领取奖励分区呈现，方便查看已有物品并找到相应入口。

[![Deep Legends 收藏界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/04-favorites.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/04-favorites.png)

### 工具 · 按自己的习惯配置客户端流程

工具页集中管理自动接受、快速下一把、断线重连等规则，并提供维护、征召、生涯和领奖入口。自动规则可以逐项开启和调整；需要修改客户端内容的操作由玩家明确触发或配置。

[![Deep Legends 工具界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/05-suite.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/05-suite.png)

### 设置 · 让窗口和信息呈现适合自己

界面缩放、密度、侧边栏、主题与默认页面都可在设置中调整；战绩与对局、隐私相关选项也按类别整理。需要分享或排查时，可从应用内选择导出位置。

[![Deep Legends 设置界面，3840 × 2160 演示截图](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/06-settings.png)](https://raw.githubusercontent.com/LLYY0418/Deep-Legends/e10ad3a3f3dfd96c7ea92825440b4aa8456b57bf/docs/r142-validation/06-settings.png)

## 安装与使用

1. 下载本页附件中带 `-public.exe` 后缀的 Windows 安装包；需要核对下载文件时，使用同页的 `SHA256SUMS-public.txt`。
2. 安装并启动 Deep Legends。未启动游戏客户端也能先浏览英雄页；登录国服客户端后，可使用总览、对局、收藏及客户端工具。
3. 公开安装包不内置个人 Riot API Key。韩服召唤师及对局等依赖 Riot 官方 API 的查询，需要使用者在运行环境中自行提供 `RIOT_API_KEY`；英雄梯度等公开数据页不依赖该 Key。

更多操作与功能边界见[项目 README](https://github.com/LLYY0418/Deep-Legends)。
