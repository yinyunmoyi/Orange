# Debug Session: statutory-long-context
- **Status**: [OPEN]
- **Issue**: 添加 statutory 的视频语境时，录制进度显示 25/55 秒，目标片段明显过长。
- **Debug Server**: <debug-server>
- **Log File**: `<workspace>/web/.dbg/trae-debug-log-statutory-long-context.ndjson`

## Reproduction Steps
1. 在阅读页选中 `statutory`。
2. 收藏或添加该单词并触发视频语境录制。
3. 观察“正在录制视频 x/y 秒”的总时长和实际等待时间。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | 句子边界识别把片头字幕和目标句合并，导致片段被计算为约 55 秒 | High | Low | **Confirmed**：日志 L1-L2 显示片头 cue 与目标句跨 20.609 秒空档仍被纳入同一片段 |
| B | 网页播放器未成功跳转到目标开始时间，从较早位置开始录制 | Medium | Medium | **Rejected**：保存的 VTT 本身从片头 cue 开始，错误在 seek 之前的片段计算 |
| C | 录制停止依赖媒体时钟，时钟卡顿或事件遗漏导致迟停 | Medium | Medium | **Rejected**：落库时长 54.890 秒与 UI 预先显示的 55 秒一致 |
| D | 55 秒来自固定兜底上限，并非字幕语境真实时长 | Medium | Low | **Rejected**：54.890 秒来自字幕首尾时间，未达到 60 秒校验上限或 64.890 秒兜底超时 |
| E | UI 将上传/转码阶段误显示为录制阶段 | Low | Low | **Rejected**：录制前 UI 已根据 `clip.durationSeconds` 显示总计 55 秒 |

## Log Evidence
Instrumentation added with `runId=pre-fix`:

- A/D/E: backend sentence range and original selection.
- A/D: matched subtitle clip boundaries and computed duration.
- B: source metadata and actual time after seek.
- C/E: recorder start state.
- C: normal media-time stop condition.
- C/D: fallback timeout.
- E: transition from recording to upload.

Pre-fix evidence:

- L1: context 117 / video 73 persisted with `durationMs=54890`.
- L2: the included title cue ends at 12.811s, dialogue starts at 33.420s, and the target cue is 43.840–47.175s.
- The title cue has no terminal punctuation, so paragraph construction joined it to the later legal sentence despite a 20.609s subtitle gap.

## Verification Conclusion
Root cause confirmed: `buildSubtitleParagraph` expands solely by character budget and ignores timeline discontinuities. It included a title/credit cue before a 20.609-second gap; the backend sentence splitter then treated that punctuation-free title and the target sentence as one sentence. The recorder correctly recorded the incorrectly calculated 54.890-second range.

Minimal fix applied: stop subtitle context expansion when adjacent cues are separated by more than 5 seconds.

Post-fix evidence:

- Post-fix log L1: the same `statutory` timeline now starts at dialogue cue 33.420s instead of title cue 0s.
- The title is excluded and the calculated clip is 21.470s, reducing recording by 33.420s.
- All 66 frontend tests pass, including a regression test based on the actual problematic timeline; production build passes.

User verification: the recording dropped to 22 seconds, confirming the timeline-gap fix, but the remaining complete legal sentence is still too long.

Iteration 2 decision: when a complete sentence exceeds 12 seconds, select a contiguous window of whole subtitle cues around the highlighted word, expanding toward nearby cues while staying within 12 seconds. Short sentences remain unchanged.

Iteration 2 post-fix evidence:

- Post-fix-2 log L1: the real `statutory` timeline is focused to 42.340–53.360s.
- Duration is 11.020s with five complete subtitle cues; the target cue remains included.
- All 67 frontend tests pass, lint reports zero warnings/errors, and production build passes.

Awaiting user verification of the 11-second recording before cleanup.
