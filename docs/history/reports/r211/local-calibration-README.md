# R211 本机采集与冻结验证

在项目根目录使用 Python 3。采集仅标准库；拟合/评估需要 numpy。Key 从 riot_key.local.txt 或 RIOT_API_KEY 读取，没配置时使用项目中转；不要把 Key 放到命令行参数或聊天。采集已使用 Mozilla/5.0，保存 HTTP 错误的去 Key 响应头与响应体。

原采集可续跑（不重新评分）：

```sh
python3 scripts/r211-collect-calibration.py --target 150 --pages 2
```

本次新集的原始采集参数（已采集完成、已经评估过，不能再叫“未见样本”）：

```sh
python3 scripts/r211-collect-calibration.py --fresh-accounts docs/history/reports/r211/new-validation-accounts.json --target 100 --pages 3
```

读取 result JSON 即可审阅结果，不要重新执行评估。r211-evaluate-new-accounts.py 在结果文件存在时拒绝二次评估；r211-refine-official-calibration.py 在候选/报告存在时拒绝覆盖冻结结果；初版 score-calibration 主入口也拒绝覆盖已冻结历史。原 score-argmax 指标是历史记录，当前官方标记/名次计算以 official 脚本为准。

下一批验证必须另建数据目录和唯一结果记录，选从未使用的源账号与对局；先冻结参数与 hash，再评估一次。不要删除当前结果文件来规避一次评估限制，不使用已评估的108局调参。
