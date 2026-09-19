# Debug Session: learning-transition-lag
- **Status**: [OPEN]
- **Issue**: 学习页点击“认识/不认识”后卡顿数秒；播放视频切片返回学习页时也卡顿数秒。
- **Debug Server**: `<debug-server>`
- **Log File**: `.dbg/trae-debug-log-learning-transition-lag.ndjson`

## Reproduction Steps
1. 在 Android App 学习页打开一个待学习单词。
2. 点击“认识”或“不认识”，观察下一题出现前的等待时间。
3. 点击视频语境切片进入播放页。
4. 返回学习页，观察页面恢复时间。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | 答题提交与下一题请求串行执行，网络耗时直接阻塞 UI 切换 | High | Low | Confirmed: L5-L9 |
| B | 提交后任务重建、同步或数据解析占用主线程 | Medium | Medium | Rejected: L7-L9 |
| C | 视频返回时学习页被重建并重新加载完整会话 | High | Low | Confirmed: L10-L21 |
| D | Media3 释放或媒体缓存检查阻塞主线程 | Medium | Medium | Rejected for return: L16-L17 |
| E | 页面恢复触发重复自动播放或语境加载 | Medium | Low | Partially confirmed: L18-L21 |

## Instrumentation Plan
- 记录答题点击、API 开始/结束、状态切换和下一题可见的时间点。
- 记录学习页进入、销毁、会话加载开始/结束。
- 记录视频页进入、播放器准备/释放、返回以及学习页恢复时间点。
- 所有事件使用 `runId=pre-fix`，通过调试服务器聚合。

## Instrumentation Status
- `WordLearningScreen.kt`: 页面生命周期、当前卡片加载、答题动画/接口/完成、视频入口。
- `ContextVideoScreen.kt`: 缓存解析、页面生命周期、返回点击、播放器释放。
- `aaa_word/biz/handler/learning.go`: 服务端答题和当前卡片处理耗时。

## Log Evidence
- L3-L4: 服务端当前卡片处理 24ms，客户端完整请求 1533ms。
- L5-L9: 答题退场动画 244ms，接口 1084ms，服务端 33ms，完整切题 1653ms。
- L10-L12: 打开视频后原学习页实例被销毁，视频页重新挂载。
- L14: 视频缓存解析 45349ms 后失败，公网媒体链路存在严重延迟。
- L15-L18: 点击返回后播放器 3ms 即释放，但学习页创建了全新实例。
- L19-L21: 返回后的当前卡片服务端仅 16ms，客户端等待 21538ms 后超时。

## Verification Conclusion
根因是 UI 生命周期和网络等待叠加，而非服务端业务处理或播放器释放：
1. 答题请求在 220ms 退场动画完成后才开始，且 HTTP 强制 `Connection: close`，公网链路耗时成为主要等待。
2. 视频页作为互斥分支替换学习页，返回必然丢失全部 Compose 状态并重新请求当前卡片。

## Fix
- 学习接口复用 HTTP 连接，并在已有 mDNS 验证端点时优先走 LAN。
- 答题请求与卡片退场动画并发执行。
- 视频页作为全屏覆盖层显示，学习页继续留在 Composition 中。

## Post-Fix Evidence
- 当前卡片：客户端 `1533ms`（返回场景最差 `21538ms` 超时）降至 LAN 下 `21ms`。
- 答题请求：`1084ms` 降至 `227ms`；完整切题 `1653ms` 降至 `581ms`。
- 连续答题样本稳定在请求 `275-283ms`、完整切题 `578-583ms`。
- 视频加载样本从 `45349ms` 失败降至 `152ms` 成功。
- 视频返回后没有出现学习页 dispose/mount，也没有重新调用 currentCard；播放器释放耗时 `19ms`。

等待用户在真实设备确认，确认前保留埋点和调试服务器。
