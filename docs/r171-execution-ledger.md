# R171 执行账本

日期：2026-09-27。基线版本 0.12.36；本轮源码版本 0.12.37。

## 数据与代码

- OPGG：沿用 `build.spellOptions[0]` 的上游顺序，在符文来源卡片级别展示一次技能组合，不把整英雄的推荐伪装成逐条符文页数据。
- 绝活哥：从对应 Riot Match V5 `riotParticipant.Summoner1ID` 和 `Summoner2ID` 透传到符文行。
- 职业选手：现有职业赛事解析结构只含装备、符文；仓库内夹具与已解析缓存不包含原始 `livestats/v1/details/{gameId}` 响应，当前环境访问该上游时 DNS 不通，尚不能确认原始响应是否提供技能字段。职业符文的两项技能 ID 保持空值，界面隐藏整行，不借用 OPGG 或其他来源。已向用户询问原始抓包路径；拿到原始响应后再按证据决定是否补解析。
- 仅在存在完整、有效且不同的两项技能 ID 时展示图标与名称和默认勾选的「同时应用召唤师技能」。取消勾选或数据缺失时请求仍只包含符文。
- 符文页成功应用后才向 `/lol-champ-select/v1/session/my-selection` 发 PATCH；PATCH 失败时响应仍为 HTTP 200、`applied:true`，并返回 `spellApplied:false`，前端提示部分成功。`perk_apply_attempt` 增加 `spell_requested` 和 `spell_applied`。

## 验证与边界

专项 Go 测试覆盖绝活哥字段透传、职业来源无数据、技能 PATCH 请求体与顺序、技能接口失败及旧请求无额外 PATCH。Node 测试覆盖三来源隔离、缺一项时整行隐藏、OPGG 卡片级展示、勾选与取消、两种成功提示。Windows 英雄选择实机写入及职业赛事原始响应仍待复核。

`go test ./backend -count=1` 全套通过（194.511 秒）；`go vet ./backend` 通过；前端全套 `node --test backend/web/*.test.cjs` 739/739 通过，`git diff --check` 通过。公开模式（**key mode: public**，编译参数中 `main.riotAPIKey` / `main.riotAPIKeyCipher` 均为空）构建 macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.37-public` 与 Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.37-public.exe`。两者源码指纹 `1ff21959b693` 均已校验，macOS `-self-test` 输出 0.12.37 并通过。未制作或发布安装包。额外启动的桌面全套测试因无关的总览渲染长耗时用例被手动停止；此前 255 项通过，未将该次中断计为通过。

## R173 / 0930 补充

R173 工单提供的真机日志判读记录两次召唤师技能应用成功（spell_applied=true、客户端 PATCH 204），OPGG / 绝活哥真机通过。职业来源技能仍为空；R173 新增 details 原始响应的安全字段形状诊断，下一份用户日志再确认字段有无，不能用既有解析结构或缓存缺字段代替原始证据。详见 r173-execution-ledger.md。
