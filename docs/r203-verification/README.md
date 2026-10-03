# R203 变异验证

基于最终源码指纹 `42989b84146c`；仅替换测试输入的生产 app.js 副本，不修改正式代码。三项变异均触发指定测试的 AssertionError，不以编译/引用错误代替行为验证。

```json
[
  {
    "name": "favorites-overlay",
    "exitCode": 1,
    "failure": "AssertionError",
    "sourceFingerprint": "42989b84146c"
  },
  {
    "name": "overlay-reset",
    "exitCode": 1,
    "failure": "AssertionError",
    "sourceFingerprint": "42989b84146c"
  },
  {
    "name": "dirty-unlimited",
    "exitCode": 1,
    "failure": "AssertionError",
    "sourceFingerprint": "42989b84146c"
  }
]
```
