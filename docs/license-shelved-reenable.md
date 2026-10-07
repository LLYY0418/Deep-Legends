# R248：注册码默认关闭与以后恢复

默认构建没有授权身份、DPAPI 授权存储、签名信任根、续租循环和业务门禁；状态 API 是 DISABLED。默认 Node 测试只发现 `.test.cjs`，注册码专项保留为 `.license.cjs`，不是 skip。CI 只跑默认构建；专项暂不放进 CI。

保留实现的完整搁置前快照：`codex/license-shelved-r232-r247`，提交 `475c20dc11cad9a0d808be02498c78c74e80d76e`。未推送。

恢复时先确认公开信任配置、服务端合同、Windows 验收及发布授权。仅加 Go 标签不自动开启桌面打包；默认桌面 `license-build.cjs` 必须保持 false。已有手动 STAGING builder 会在临时副本中同时设置标签和桌面标记，正式脚本始终默认关闭。

显式检查命令：

```sh
go test -count=1 -tags license -run 'R232|R233|R234|R236|R237|R240|R242|R243|R245' ./backend
go test -count=1 -tags license,license_staging -run 'R233|R236|R237' ./backend
(cd installer && go test -count=1 -tags license ./...)
DEEP_LEGENDS_TEST_LICENSE=1 node scripts/test-license.cjs
DEEP_LEGENDS_TEST_LICENSE=1 node scripts/r240-electron.cjs acceptance
DEEP_LEGENDS_TEST_LICENSE=1 node scripts/r240-browser.cjs
# 手动构建留待未来授权；本轮不执行。
node scripts/build-license-staging.cjs --output dist/future-staging-public
```

installer 是独立 Go 模块；安装器检查实际应在 `installer/` 中运行 `go test -count=1 -tags license ./...`。浏览器/Electron 夹具需使用带 license 标记的待验桌面源码；不要把 disabled 的默认源码当授权验收包。

R246 已停止，69 条新共享向量保留。两小时测试 issuer 与 R242 的 900 秒授权到期旧夹具有已知不一致，按 R248 只记录，不继续修。恢复前应单独完成 R246 的夹具与正式验收，不能取消“租约晚于授权到期必须拒绝”或“租约过期必须联网续租成功”的规则。

默认更新恢复 0.12.76：清单 schema/版本/文件名/大小/SHA-256/固定 GitHub 发布地址等既有检查，下载和应用前仍校验安装包大小与 SHA-256；不要求 R232 的 Ed25519 更新清单 envelope。带 license 的更新验签保留，STAGING 仍关闭在线更新。
