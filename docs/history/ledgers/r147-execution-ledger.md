# R147 执行账本

日期：2026-09-24。基线：0.12.19，工作区包含 R144–R146 未提交改动。

## 范围与判断

- 用户明确要求评估工单逻辑并直接执行，许可问题无需继续核对。
- [探测结论](../../r147-probe-findings.md)：页面会直接下发 `hexpage`，现有三个 API 带 Cookie 时均有成功样本；响应仍偶发 `page_token_required`，所以只能保证有限重试和可靠降级，不能保证每次恢复。
- 保留 API 路径；没有新增静态 JSON 白名单。`/heroes` HTML 解析失败另列，不在 R147 顺手修改。

## 代码

- `backend/hexdata.go`：Hexdata 专用内存 Cookie jar，仅接受 `hexdata.com.cn` 主机及 `hexpage` / `ink` 名称；当前英雄页面取令牌；全局节流与并发单飞；按页面 `Max-Age` 提前刷新；令牌 403 最多重取与重试一次；仅最终失败进入原有熔断流程。诊断只记状态、耗时、类别和错误码，不记 Cookie 值。
- `backend/hexdata_token_test.go`：假上游覆盖初次取页、主动过期、刷新上限、十路并发和秘密不落诊断/磁盘。既有冷启动请求预算更新为增加一次英雄页面请求。

## 验证

- `go build -o /tmp/deep-legends-r147 ./backend`：通过。直接 `go build ./backend` 因仓库已有 `backend/` 目录，Go 拒绝同名输出；指定临时输出路径后完成真实可执行文件构建。
- `go vet ./...`：通过。
- `go test ./... -count=1`：通过（约 198 秒）。首次全量运行只发现 R116 旧断言要求英雄页请求数为 0，已按 R147 改为 1 后重跑通过。
- `go test -race ./backend -run 'TestHexdataPageToken|TestHexdataTokenJar|TestLoadMayhemDetailColdHero' -count=1`：通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：960 项，959 通过、1 跳过、0 失败（约 247 秒）。
- 六项对抗变异均被对应测试抓到：删除令牌重取、超出一次重试、关闭单飞、诊断写入 Cookie、恢复成功仍计熔断失败、放行英雄列表路径。每项测试变红后均恢复原文件。
- 当前 Go 客户端的真实站点单英雄核验：英雄 887 JSON、insights、postmatch 三个 API 在同一内存 Cookie jar 下均返回 200；临时核验测试已删除。此前探测存在偶发 403，仍需用户装包后观察。
- 真机界面验收仍需新包与用户日志；用户此前要求暂不改版本或打包，本轮保留 0.12.19。
