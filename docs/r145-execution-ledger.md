# R145 执行账本（进行中）

版本保持 `0.12.19`。本轮复核 `backend/main.go` 的 `/fe/` 图标到 CommunityDragon 的映射和 `backend/loot_icon_fallback_test.go` 两条新增测试；按用户要求只做代码与验证，暂不打包。

## 已验证

- `go build -o /tmp/r145-backend-verify ./backend`、`go vet ./...`、`go test -count=1 ./...` 在真实 Go 依赖下通过；全量 Go 测试耗时 203.769 秒。
- 两条 R145 新测试及既有的图片路径回归测试定向通过；`node --test backend/web/*.test.cjs` 694/694 通过。`git diff --check` 与 R145 Go 文件的 `gofmt -l` 均无问题。
- 2026-09-24 用只读 HTTP HEAD 检查 `raw.communitydragon.org` 的 `chest_128.png` 和 `material_clashtickets.png`：两者均返回 200、`content-type: image/png`，长度分别为 41926 和 16253 字节。运行时图片回落由本地模拟 LCU 404 与镜像 200 的 Go 测试覆盖；仍需真机确认最终界面。

## 待办

- 版本递增、private 安装包、key mode、构建指纹及 SHA-256 随发版记录；本轮未打包，不能填写安装包数值。
- Windows 真机检查两个图标是否显示，并导出诊断日志确认 404 回落与约 10 秒图片等待是否消失。未用相关性代替因果结论。
