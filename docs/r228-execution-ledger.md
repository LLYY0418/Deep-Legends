# R228 执行账本

2026-10-05。用户要求“先按照这个工单继续执行”。目标0.12.74，key mode **public**。

工单：[WORKLIST-R228](WORKLIST-R228-RELEASE-0.12.74-CONSOLIDATE-R223-R227-AND-R222-R225-PIPELINE.md)。当前状态：执行中；正式Latest未发布，等待本单P6用户明确确认。

## 接手时的事实

工单所述未提交状态已在此前“发布新版本”指示下推进：源码及版本合并提交`434caa922526f483277c15e9f722db2b9d4da289`，tag v0.12.74已于22:59推送，草稿id403843482。草稿37329122050已success，指纹a3d7c1e75735；完整质量37329122051随后于23:08:10结束并success。接手时未发布Latest；立即按P6收窄为草稿准备。

本地预检在版本变更前完成，Go/测试源码快照无变动；10组检查全过，1280 Node项1276通过/4平台跳过。原始日志与四个旧验证目录未提交，凭据扫描无Riot Key/GitHub token/私钥命中。R225文件内容已逐文件同步，但原提交祖先缺失；本轮补齐祖先与分支CI。

版本混合提交、tag先于本单分支CI是R228到达前的真实流程偏差，无法补写成先分支CI后版本提交；不移动现有tag，不假冒另一次tag验证。本轮另补完整分支CI，同步用户说明与草稿元数据，保留两个SHA的证据边界。

## 发布边界

保持草稿，不执行`gh release edit --draft=false --latest`。R223/R224真实Windows游戏客户端验收保持待验。Worker源未改，无新部署、不读取或替换Secret。压缩等级9保留。

## P1/P2补证与P3分支CI

R225全部9个提交已合入祖先，合并提交 `0d6b760dae805470130a93f85fabbf8f62b76339`。两份冲突仅为索引与R222账本的附加记录，保留R225纠正与R226/R228记录。`git diff v0.12.74 -- backend desktop scripts installer relay .github`为空，源码/测试/构建指纹完全一致；因此之前最终源码本地预检仍适用，没有新源码或夹具修改。

分支CI [37330990603](https://github.com/LLYY0418/Deep-Legends/actions/runs/37330990603) 于23:12:55创建，等待完整结论。现有tag质量 [37329122051](https://github.com/LLYY0418/Deep-Legends/actions/runs/37329122051) success：Linux race115.133s、Node157.798s/最大文件49.155s；Windows backend1880顶层项1854通过/26既有缺样本或opt-in跳过，关键82项均通过；installer99/99；Node筛选2/1/2均0skip。实际升级0.12.65→0.12.68→0.12.74，8阶段、9.882s，卸旧1.538s，copy1ms，图标位置和快捷方式创建时间保持。

新增产品Go测试19项（R223=12、R224=5、R227=2）在Windows全部PASS，采用Go输出的四舍五入秒数，不用日志传输时间戳冒充执行耗时。新增产品Node顶层13项（R223=7、R224=5、R226=1），R225静态护栏另5项。Node文件R223/R224/refresh分别1.316/1.713/8.712s，均未越预算。[CI验证汇总](history/reports/release-0.12.74/ci-validation-summary.json)。

本机4项skip：R82 receipt lookup、R82 filename/hash mutations（无PowerShell）；R86 Windows failing-Go gate、R222 Windows local skip rejection（非Windows）。Windows中对应项已实际通过，未删除/放宽断言或扩大skip。root Go/race/vet、installer、Worker15、Chromium R100峰值5/R117无未样式帧及public重建/自检均通过。

P7可照做清单：[Windows真机检查清单](r228-windows-client-checklist.md)。未获得用户真机证据，没有声明国服真实账号已恢复，R223/R224保持待验。

## 首轮分支预算未达标，按P2修复

分支37330990603完整软件检查success，但Linux backend race **139.721s >120s**，不能写P3全部门槛通过；Node222.657s、最大external70.790s均通过。该SHA生产源码与tag完全相同，实际机器耗时存在差异，未用tag115.133s替代此轮失败预算。

Windows顶层耗时定位R69状态测试21.00s：R223新增未知大区LCU重试后，这个旧夹具不设大区，HTTP500真实等待3+6+12秒。修复只在`TestR69LiveHistoryStatesDistinguishUnavailableEmptyAndFailed`注入既有historyRetrySleep，保留未知大区、实际四次失败请求和原状态/缓存断言，新增空响应不等待、失败恰好3/6/12s的断言。生产默认时钟、请求逻辑和已冻结public指纹不变；没有删除、缩减数据或放宽时限。其余已并行等待测试未改；R99含t.Setenv，不盲加并行。

最后一次Go测试修改后重跑普通全量/race/vet及R69/R223定向race，再执行完整分支CI。旧轮失败门槛和真实时长保留。

最终Go夹具修改后，本机定向race三轮通过（包1.874s）；普通全量23:30:31→23:32:27，115.116s，backend107.053s；全量race23:32:27→23:34:44，137.349s，backend135.613s（macOS，不能代替Linux≤120）；vet23:34:44→23:34:46，2.039s，输出空、exit0。原生public后端重新构建、self-test和指纹核验通过，0.12.74/a3d7c1e75735。原有Node/Worker/installer/Chromium源码与测试未变，其预检结果仍对应最终源码。[完整最终Go预检摘要及输出末5行](history/reports/release-0.12.74/r228-go-preflight-after-fix.json)。
