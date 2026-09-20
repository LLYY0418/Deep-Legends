# R105 验证索引

完整要求映射和Windows待验项：[执行账本](../r105-execution-ledger.md)。

| 证据 | 结果 |
|---|---|
| go-full.txt | 最终生产源码全量Go测试通过，包含R95–R105回归 |
| go-race.txt | R105 race通过 |
| go-build.txt / go-build-windows.txt / go-vet.txt | 原生/Windows构建及vet；成功时日志为空 |
| node-full.txt | web + desktop共731项：730通过、1平台跳过、0失败 |
| node-targeted.txt | 51项通过 |
| pro-players-response.json / .txt | 真实handler旧快照迁移结果：33人53账号；同时核对pro_players诊断事件 |
| mutations/matrix.json | 13项均有通过baseline并捕获变异；逐项原始输出在同目录 |
| package-build.txt | 0.12.2完整安装包构建与包内后端/asar校验 |
| artifact-verification.json | 完成后再次核对源码指纹、文件哈希与嵌入的R105实现 |

这些离线自动化结果不代表Windows游戏客户端实测。用户工单提供的W5/W6 201/200及恢复结果已单独归档至 `../r99-probe-results.md`，本次未获得原始20:38日志。
