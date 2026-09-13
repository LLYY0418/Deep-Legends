# WORKLIST-R82-ADDENDUM-2 · A/B 采集脚本文件名假设过期，改成读构建记录

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。

## 问题

用户在真机上按 R82 主工单跑 `scripts/r82-startup-ab.ps1` 时报错：

```
.\r82-startup-ab.ps1 -Installer 'D:\Download\Deep Legends Setup 0.12.1.exe' -Group control
Use the original Deep Legends Setup <fingerprint>.exe filename; do not rename an old build.
```

**根因**：`scripts/r82-startup-ab.ps1:13` 校验安装包文件名必须匹配
`^Deep Legends Setup ([0-9a-f]{12})\.exe$`（文件名里要带 12 位十六进制指纹），
这是我们在 R82 主工单/补充单那几轮里对齐的旧规则。但仓库里**同一天更早的时候**
（`desktop/package.json`、`installer/build-shell.cjs` 大约在 12:35~12:59 被改过），
打包产物的命名规则已经变成**只用版本号**：`Deep Legends Setup <version>.exe`
（`installer/build-shell.cjs:67` 现在是 `Deep Legends Setup ${version}.exe`，
`build-desktop-windows.ps1:129` 也有注释"文件名使用版本号；诊断、构建核验及
portable 内部缓存仍保留源码指纹"）。也就是说**当前真实产物的文件名里已经没有指纹了**，
这条脚本的假设过期了，不是用户的包有问题。

**好消息**：指纹并没有真的丢，`dist/desktop/release-build.json`（每次构建都会生成，
和最终产物同目录）里记着 `version`、`fingerprint`，以及每个产物文件名到其
SHA-256 的映射（`assets` 字段）。这才是当前唯一可靠的"文件名 → 指纹"查找路径。

## 要做什么

修改 `scripts/r82-startup-ab.ps1`，把"从文件名正则提取指纹"换成
"从同目录的 `release-build.json` 按哈希反查指纹"：

1. 不再要求 `-Installer` 参数指向的文件名必须匹配任何固定格式（用户以后可能
   还会再改命名规则，脚本不该绑死在某一种文件名形状上）。
2. 找同目录（`Split-Path -Parent $setupPath`）下的 `release-build.json`，读取它。
   找不到这个文件就明确报错："找不到 release-build.json，无法确认这是不是一次
   全新构建；把整个 dist/desktop 目录一起带过来，不要只拷贝安装包本身"。
3. 计算 `$setupPath` 的 SHA-256（`Get-FileHash -Algorithm SHA256`），
   在 `release-build.json` 的 `assets` 字段里按**文件哈希**（不是文件名）反查——
   遍历 `assets` 的 value，找到哈希相等的那一项，取 `release-build.json` 顶层的
   `fingerprint` 字段作为这次构建的指纹。
   **按哈希而不是按文件名去匹配 `assets`**，因为用户完全可能把文件重命名过
   （比如加个日期后缀方便自己归档），文件名不可靠，内容哈希才可靠。
4. 如果算出来的哈希在 `assets` 里**找不到匹配项**，说明这个 exe 和
   `release-build.json` 不是同一次构建产出的（比如用户把旧 exe 和新的
   `release-build.json` 混在一个目录里），报错并终止，不要猜测着继续跑。
5. 后续逻辑（"这个指纹是否已经测过"的去重检查、传给
   `r82-startup-ab-report.cjs` 的指纹参数等）全部不变，只是指纹的**来源**从
   "解析文件名"换成"查构建记录 + 校验哈希"。
6. **顺带检查** `r82-startup-ab-report.cjs` 里有没有同样对文件名格式做假设的地方
   （比如 `validateIdentity` 里的指纹正则本身不用改，那个是校验指纹字符串格式，
   跟文件名无关；但如果发现别的地方也在假设文件名形状，一并修掉）。

## 验收判据

1. 用一个文件名为 `Deep Legends Setup 0.12.1.exe`（当前真实命名规则）的构建产物
   + 同目录的 `release-build.json` 去跑脚本，**不再报错**，能正确提取出指纹并继续。
2. 故意把 exe 换成另一个不相关的文件（内容不匹配 `release-build.json` 里任何一项
   哈希），脚本必须报错终止，不能装作若无其事地继续用一个错误的指纹跑下去。
3. 缺 `release-build.json` 时报错文案清晰，用户看得懂要怎么补救（见上面第 2 条的措辞）。
4. 原有"同一指纹不能重复测量"的去重逻辑必须保持有效——测两次同一个
   `release-build.json`+exe 组合，第二次必须仍然报错拒绝。

## 变异判据

| 变异 | 测试/手动验证必须能发现 |
|---|---|
| 反查时按文件名而不是按哈希匹配 `assets` | ✅（用改名后的同一个文件测试，应该仍能正确匹配） |
| 哈希对不上时不报错、继续用第一个 assets 项的指纹 | ✅（故意传错文件，必须报错而不是静默用错指纹） |
| 去重检查在换成读 `release-build.json` 后失效 | ✅（同一指纹重复测两次必须仍被拒绝） |

## 不要做的事

- 不要把 R82 主工单/补充单里其它任何逻辑一起改动，这单只修这一个文件名假设过期的问题。
- 不要反过来把 `installer/build-shell.cjs` 的命名规则改回带指纹——命名规则改成
  版本号是仓库里已经发生、看起来是有意为之的改动（`build-desktop-windows.ps1:129`
  有专门的注释说明），这单不评估那个决定对不对，只让 A/B 脚本适配现状。
