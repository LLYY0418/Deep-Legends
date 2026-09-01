# WORKLIST 0823-R14（打包卫生：go:embed 别把测试/开发文件带进生产 exe）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
优先级：P1（不影响功能，但是真实的信息泄露面，且已经在 0823 打包审计中被字符串扫描证实）。

---

## 问题

`main.go:33`：

```go
//go:embed web/* data/reroll_pool_14_5.txt data/reroll_pool_14_5.json
var embedded embed.FS
```

`web/*` 是不分文件类型的通配，把整个 `web/` 目录树全部编进了二进制，包括三个明确不该随包分发的开发文件：

- `web/champions.test.cjs`（122KB，测试断言 + mock 数据结构）
- `web/remaining-sort.test.cjs`（10KB）
- `web/loot-icons/README.md`

而 `main.go:239-248` 又用 `http.FileServer(http.FS(webFS))` 把这个 embedded FS **整个当静态站点托管**：

```go
webFS, err := fs.Sub(embedded, "web")
...
fileServer := http.FileServer(http.FS(webFS))
```

**后果**：程序正常运行时，`http://127.0.0.1:<端口>/champions.test.cjs` 是能被外部直接请求下载的
（已用真实打出的 `Deep Legends.exe` 做过 `strings` 扫描证实：
`web/champions.test.cjs`、`web/remaining-sort.test.cjs` 两个文件名字符串原样出现在二进制里）。
测试文件里的断言、mock 数据结构、内部字段命名规则会被暴露。

**不是**权限问题（本地服务本来就只监听 loopback），是打包卫生问题：测试代码不该出现在生产制品里，
且没有任何东西在把关"新增到 `web/` 下的文件默认就会被打进包"这件事。

---

## 修复方案：把 catch-all 换成按用途显式列举

**不要**把测试文件移出 `web/` 目录——`web/champions.test.cjs`、`web/remaining-sort.test.cjs`
的路径被 `.github/workflows/ci.yml:33`、`README.md:142` 等多处直接引用，移动会牵连 CI 和文档。
**只改 `go:embed` 这一行**，物理文件位置不动。

`web/` 目录当前实测内容（子目录只含图标资源，无例外）：

```
web/            *.js *.css *.html          （运行时需要）
                *.test.cjs                  （测试代码，2 个文件，不该打包）
web/arena-team-icons/   *.svg
web/position-icons/     *.svg
web/tier-icons/         *.svg
web/rune-styles/        *.svg
web/loot-icons/         *.svg *.png *.md   （README.md 不该打包）
web/rank-crests/        *.png
```

`main.go:33` 改成按扩展名显式列举（Go 允许多条 `//go:embed` 指令共同作用于同一个变量）：

```go
//go:embed web/*.js web/*.css web/*.html
//go:embed web/arena-team-icons/*.svg web/position-icons/*.svg web/tier-icons/*.svg
//go:embed web/rune-styles/*.svg web/loot-icons/*.svg web/loot-icons/*.png web/rank-crests/*.png
//go:embed data/reroll_pool_14_5.txt data/reroll_pool_14_5.json
var embedded embed.FS
```

这样 `*.test.cjs` 和 `*.md` 因为扩展名不在任何一条 pattern 里，自然不会被收进 `embedded`，
不需要任何"排除逻辑"。以后 `web/` 下新增的 `.js/.css/.html/.svg/.png` 文件会继续被自动收录；
新增其它类型文件默认**不会**被打包，需要显式加一条 pattern 才行——这是有意的：
"新文件默认不进包"比"新文件默认进包"更安全。

---

## 护栏（这条必须有，本项目反复出现"看似有效实则零覆盖"的教训）

在 `main_test.go`（或新建 `embed_test.go`）里加一条测试，做两件事：

1. **正例**：断言 `embedded` 里确实能读到几个已知的运行时文件
   （如 `web/champions.js`、`web/index.html`、`web/rank-crests/diamond.png`），
   防止有人手滑把某个真正需要的目录漏掉。
2. **反例（本单的核心目的）**：断言 `embedded` 里读不到
   `web/champions.test.cjs`、`web/remaining-sort.test.cjs`、`web/loot-icons/README.md`——
   `fs.ReadFile` 应该返回 `fs.ErrNotExist`。

```go
func TestEmbeddedFilesystemExcludesDevOnlyFiles(t *testing.T) {
    for _, present := range []string{
        "web/champions.js", "web/index.html", "web/rank-crests/diamond.png",
    } {
        if _, err := embedded.ReadFile(present); err != nil {
            t.Fatalf("expected %s to be embedded, got %v", present, err)
        }
    }
    for _, absent := range []string{
        "web/champions.test.cjs", "web/remaining-sort.test.cjs", "web/loot-icons/README.md",
    } {
        if _, err := embedded.ReadFile(absent); !errors.Is(err, fs.ErrNotExist) {
            t.Fatalf("expected %s to be excluded from the embedded binary, got err=%v", absent, err)
        }
    }
}
```

**用变异体自验**：把 `go:embed` 改回 `web/*` 一行，这条测试必须失败（fail 掉才说明是真护栏，
不是形式主义）；改完再测一次确认变回显式列举后测试通过。

---

## 验收清单

- [ ] `main.go:33` 的 `go:embed` 从 `web/*` 改成按扩展名显式列举，物理文件一个不动
- [ ] 新增 `TestEmbeddedFilesystemExcludesDevOnlyFiles`，正例 + 反例都覆盖
- [ ] 变异体自验：改回 `web/*`，新测试必须失败；改回显式列举，必须通过
- [ ] 打一次真实 `./build-desktop.sh`，对新产物的 `backend/loot-service.exe` 跑
      `strings -a loot-service.exe | grep -c "champions.test.cjs"`，结果必须是 `0`
      （R13 验收时我是用这条命令实测发现问题的，改完请用同一条命令自证）
- [ ] `go test ./...` 全绿，`gofmt -l` 只剩既有的 `yourgg_arena.go`
- [ ] 确认前端功能不受影响：`web/champions.js`、`web/index.html`、图标目录仍能正常加载
      （随便打开一次英雄详情页，图标应正常显示，不是破图）
