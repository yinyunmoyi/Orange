# Debug Session: s05e16-subtitle-sync
- **Status**: [OPEN]
- **Issue**: S05E16 在 1:16 前字幕与视频偶发不同步；点击字幕块有时恢复、有时不恢复。
- **Debug Server**: Pending
- **Log File**: .dbg/trae-debug-log-s05e16-subtitle-sync.ndjson

## Reproduction Steps
1. 在 Web 视频管理中打开 `SHDBZ S05E16.mkv` 与对应 SRT。
2. 播放并观察 0:00 至 1:16 的字幕高亮与台词。
3. 点击前段任意字幕块，观察视频实际跳转位置和高亮是否一致。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | 前段重叠 SRT 时间块使单一高亮选择不稳定。 | High | Low | Pending |
| B | 兼容播放器首段媒体时间轴与 SRT 零点存在固定偏移。 | Medium | Medium | Pending |
| C | 点击字幕后，HLS 分片按关键帧回退，实际时间未落在目标时间。 | Medium | Low | Pending |
| D | DTS 转码首段造成音画时间戳偏移。 | Low | Medium | Pending |

## Log Evidence
- `trae-debug-log-s05e16-subtitle-sync.ndjson`: 点击 `1-0` 请求 `4.06s` 后，播放器在 `4.059999s` 完成 seek，缓冲区为 `0–51.635002s`。
- 同一日志在 `12.233973s` 高亮 `5-4`，`14.09316s` 又切换为 `6-5`；两条分别覆盖 `12.16–16.69s` 与 `14.03–16.69s`，存在重叠。
- SRT 共 448 条 cue、5 组重叠；前段两组为 `12.16–16.69 / 14.03–16.69` 与 `16.69–20.69 / 18.34–20.69`。

## Verification Conclusion
- A: Confirmed. 现有 `findHighlightedCue` 只选择最新开始的一条，重叠期间遗漏另一条有效 cue。
- B: Rejected. 媒体 metadata 从 `0s` 开始，播放时钟与 SRT seek 目标一致。
- C: Rejected. 点击 `4.06s` 后实际落点 `4.059999s`，没有关键帧回退。
- D: Rejected. 偏差与含 DTS 的音频时间轴无关，问题发生在字幕高亮选择。

## Post-fix Verification
- `findActiveCues()` 在 `14.5s` 对重叠 cue 返回 `5-4` 与 `6-5`，两个字幕块都会获得激活样式；自动滚动仍只使用原有主 cue，避免列表行为变化。
- 完整视频模块测试通过（77/77），`npm run lint` 通过，生产构建通过。
- 浏览器自动化确认播放器实际时间持续前进；但字幕文本区域的点击会按既有交互规则被忽略，不能据此作为 seek 失败证据。实际点击字幕块留白区域会触发 seek。

## Audio/Video Follow-up
- 用户确认修复字幕重叠后，S05E16 仍存在音画不同步。
- 原始文件的首个视频与音频包都从 `0.083s` 开始，排除源文件存在单一固定音画偏移。
- 已确认 `playsvideo@0.4.7` 的分片逻辑允许视频回退到任意早的关键帧，却从分片起点提取音频；当关键帧回退时，同一分片中的视频和音频前导区间不一致。
- 已应用上游 `b705fb31` 的边界修复并以 `patch-package` 固化：视频关键帧最多回退 `0.5s`；若仍有视频前导，音频会从同一前导点开始，最多保留 `0.75s`。
- 已通过干净 `npm ci` 验证补丁会自动应用。新实例实播至 `68.7s`，视频未暂停、缓冲连续至 `100.4s`、音频解码字节增长至 `1,377,833`，视频丢帧为 `0`。

## Audio Gap Follow-up
- 用户反馈部分台词被直接跳过，之后音频、画面和字幕均无法对齐。
- 已增加应用层播放时钟观测：媒体时间相对墙钟异常前跳、视频前进但音频解码字节不增长、以及 `waiting` / `stalled` / `seeking` 都会上报到当前调试会话。
- 原先关键帧兼容补丁会在回退超过 `0.5s` 时切换到下一关键帧，这会主动跳过分片开头的内容，和本次症状相符。
- 已改为始终从实际视频关键帧开始，并让 DTS 音频从该相同前导点开始转码；不再限制音频前导时长，也不再向前跳到下一关键帧。
- 已重新验证补丁可由 `postinstall` 自动应用；完整测试（77/77）、lint 与生产构建均通过。
