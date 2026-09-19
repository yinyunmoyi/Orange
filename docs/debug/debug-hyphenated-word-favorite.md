# Debug Session: hyphenated-word-favorite
- **Status**: [OPEN]
- **Issue**: 字幕词 `by-elections` 的弹窗中英文释义加载失败，收藏按钮不可点击。
- **Debug Server**: <debug-server>
- **Log File**: `.dbg/trae-debug-log-hyphenated-word-favorite.ndjson`

## Reproduction Steps
1. 在视频字幕中选中 `by-elections`。
2. 打开单词查询弹窗。
3. 等待释义请求结束并点击右上角收藏图标。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | 英文词典请求瞬时失败，`english.status=error` 导致收藏条件失败 | High | Low | **Confirmed**：L1 显示英文失败时按钮禁用；L2 显示当前重试已恢复 |
| B | 收藏状态请求失败，`favorite.status` 未进入 success | Medium | Low | **Rejected**：L2 返回 200 且未收藏 |
| C | 前端将 `by-elections` 错误分类为不支持的类型 | Low | Low | **Rejected**：L1-L2 均识别为 word |
| D | 后端不允许缺少英文词典数据的降级收藏 | Medium | Low | **Confirmed**：L3 返回 500 `favorite: missing word data` |
| E | 收藏按钮可用但被弹层或其他元素遮挡 | Low | Low | **Rejected**：L1 明确为 disabled |

## Log Evidence
Instrumentation added with `runId=pre-fix`:

- A/C: `/word/lookup` HTTP status, response code and data presence.
- B: favorite-status HTTP status and result.
- A/B/C/D/E: resolved popup state and exact `canFavorite` gate inputs.

Pre-fix evidence:

- L1: screenshot state has `englishStatus=error`, while Chinese meaning and AI explanation succeeded; the favorite button is disabled.
- L2: word classification, lookup route, meaning route, and favorite-status route all currently return 200, indicating a transient lookup failure rather than an unsupported hyphenated word.
- L3: backend rejects the safe fallback request when `wordData` is absent, so frontend-only gate removal is insufficient.

## Verification Conclusion
Root cause confirmed: favorite eligibility is unnecessarily coupled to one English dictionary request. A transient failure permanently disables the button for the popup session, and the backend currently cannot synthesize fallback word data for the otherwise valid word.

Initial fallback-at-favorite approach was rejected by the user and reverted.

Revised fix:

- Keep the original favorite gate: it still requires a successful `/lookup` response.
- The dictionary service performs up to three attempts with 250ms and 750ms retry delays.
- Only after all attempts fail does `/lookup` return a TTS-only `WordData`, allowing the normal favorite request to proceed.

Post-fix evidence:

- The rejected implementation's temporary integration item 7729 was removed and its status returned to `favorited=false`.
- Post-fix log L1: retry policy is three attempts with 250ms and 750ms delays; exhausted failures return one US TTS pronunciation.
- Post-fix log L2: after restart, the real `by-elections` lookup returns code 0 with its noun definition and US pronunciation.
- Go tests pass, including the exhausted-retry fallback case; all 67 frontend tests, lint, and production build pass.
- Backend was restarted on port 8888 with the revised behavior.

Awaiting user verification before removing instrumentation and debug artifacts.
