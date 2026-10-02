# R172 执行账本

日期：2026-09-27。基线版本 0.12.36；本轮源码版本 0.12.37。

## 代码与验证

经典模式每条核心装路线前均加入首组选出的出门装 ID；出门装之后用 3px 的主题色竖条分隔，后续核心装仍保留原箭头。顶部独立的「出门装」区块保留。没有出门装数据时，路线不增加图标或竖条。斗魂竞技场和海克斯大乱斗不接入这段经典模式路线前缀。

Node 测试逐项验证出门装、竖线、核心装及箭头的顺序；检查无出门装时的兼容输出，以及斗魂和海克斯大乱斗分支。Windows 视觉效果仍待真机复核。

`go test ./backend -count=1` 全套通过（194.511 秒）；`go vet ./backend` 通过；前端全套 `node --test backend/web/*.test.cjs` 739/739 通过，`git diff --check` 通过。与 R171 共用本轮 0.12.37 public 验证构建：macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.37-public`、Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.37-public.exe`；源码指纹 `1ff21959b693` 均已校验，macOS 自检通过。未制作或发布安装包。额外启动的桌面全套测试因无关的总览渲染长耗时用例被手动停止；此前 255 项通过，未将该次中断计为通过。
