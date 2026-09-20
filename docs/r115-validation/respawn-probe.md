# R115 复活时间探针边界

日期：2026-09-19。状态：**未执行 Windows/真实对局探针，P3-A 暂缓。**

`arena_truth_diagnostics.go` 的 `respawnTimer` 白名单和测试桩只证明代码认识该字段，不能证明真实客户端在各模式、死亡/复活切换时持续提供正确秒数。本轮没有真实 allgamedata 样本，不填造探针值。

`gameplay_refresh.go:cachedGameplayLive` 对完整 InProgress/Reconnect 快照按整局缓存；其余通常缓存 20 秒，特定过渡阶段为 3 秒。这条数据链面向阵容信息，不能直接成为复活倒计时时钟。局部 UI 每秒递减仍需可靠初值和校正来源，否则会显示推断值。

本轮未新增 2999 轮询、Live Client Data 请求、复活浮窗或相关设置。后续需实际验证字段单位、缺失语义、不同模式及重连行为，再确定是否有可靠且不会增加高频请求的实现方案。
