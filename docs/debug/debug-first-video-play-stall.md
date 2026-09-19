# Debug Session: first-video-play-stall
- **Status**: [OPEN]
- **Issue**: Web 端第一次选择视频进入播放页后卡住；退出后再次进入同一视频可正常播放。
- **Debug Server**: <debug-server>
- **Log File**: `.dbg/trae-debug-log-first-video-play-stall.ndjson`

## Reproduction Steps
1. 打开 Web 端视频列表。
2. 第一次选择一个尚未进入过的视频。
3. 进入播放页后观察播放器卡住。
4. 退出播放页。
5. 再次进入同一个视频，观察其恢复正常播放。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | 首次进入时视频文件的 Object URL 或 IndexedDB 数据尚未准备好，播放器先于资源完成初始化 | High | Low | Pending |
| B | React Strict Mode 首次挂载清理了播放器资源，后续 effect 没有完整重建 | High | Medium | Pending |
| C | 首次能力探测或 HLS 回退流程一直等待未触发的媒体事件 | Medium | Medium | Pending |
| D | 首次 `play()` 被浏览器自动播放策略拒绝，二次进入因交互状态变化而成功 | Medium | Low | Pending |
| E | 视频原生播放无音频，首次 HLS 冷转码耗时较长，退出后转码继续完成，二次进入复用结果 | High | Low | Pending |

## Instrumentation Plan
- 记录播放器页面挂载、卸载以及视频资源元信息。
- 记录 Object URL 创建、绑定和回收时序。
- 记录媒体关键事件、`readyState`、`networkState`、错误和当前时间。
- 记录能力探测、原生播放与 HLS 回退的开始、完成和失败。

## Log Evidence
- `data/debug/frontend.log:9647-9656`：17:08:11 首次进入；原生视频成功 `play:resolved`，但 1.2 秒探针显示 `audioDecoded=0`、`videoDecoded=677091`，以 `reason=no-audio` 转入 HLS。
- `data/debug/frontend.log:9657`：17:08:21 用户退出，距离 HLS 开始约 9.2 秒，此时还未生成播放清单。
- `data/debug/frontend.log:9658-9667`：17:08:22 再次进入，仍先检测到原生无音频并等待相同 HLS 任务。
- `data/debug/frontend.log:9668-9694`：17:08:32 HLS 清单和首段可用，17:08:34 探针通过并完成挂载；首次 HLS 准备总耗时约 21.7 秒。
- HLS 缓存目录中 `master.m3u8` 的落盘时间为 17:08:32，当前 status 接口返回 `ready=true`，与前端日志时序一致。
- 两次进入均出现同毫秒 `effect:start -> effect:cleanup -> effect:start`，确认开发环境 React Strict Mode 触发了双 effect；第二个 effect 正常完成，因而它不是“退出再进入才播放”的主因，但会产生一次无效探测并干扰共享 video 元素。
- 两次 `play:resolved` 均为 `outcome=played`，没有 `play:autoplay-blocked` 或 `play:rejected`。

## Verification Conclusion
- A Rejected：Object URL 在首次进入时已加载出 metadata 和完整 duration。
- B Confirmed as contributing issue：Strict Mode 双挂载存在，但有效 effect 可继续执行，不是本次二次进入成功的决定因素。
- C Rejected：媒体事件和 HLS 事件均正常触发；等待的是后端冷转码完成，不是丢失事件。
- D Rejected：自动播放成功，没有浏览器策略拒绝。
- E Confirmed：根因是原视频音轨不兼容触发 HLS 冷转码，用户在约 9 秒时退出；转码在约 21 秒时完成，二次进入复用了同一个进行中任务/缓存。

## Fix
- 前端准备状态细分为兼容性检测、缓存检查、上传、转码和加载视频。
- 前端上传视频时携带源视频时长，并每 500ms 查询后端转码状态。
- 同一 fingerprint 的进行中转码改为多订阅者模型，退出后重新进入仍能收到当前进度。
- Go 后端解析 FFmpeg `out_time`，按源视频时长计算 0-99 的真实转码百分比；完成后返回 100。
- 播放适配器 `attach` 返回后立即检查取消信号；已取消任务不再进入 1.2 秒媒体探针。

## Post-fix Verification
- Web `npm run build`：通过。
- Web `npm run lint`：通过，0 warning / 0 error。
- Strict Mode 取消回归测试：通过，确认取消发生在异步 attach 后时不会注册探针监听器。
- Go 新增进度状态和 FFmpeg 时间解析测试：通过。
- Go 全包仅编译验证：通过。
- 真实冷转码运行验证：等待重启当前 Go 服务后执行。
