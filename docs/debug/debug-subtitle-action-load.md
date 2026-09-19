# Debug Session: subtitle-action-load
- **Status**: [OPEN]
- **Issue**: Video player subtitle lookup and sentence analysis actions never finish loading.
- **Debug Server**: <debug-server>
- **Log File**: .dbg/trae-debug-log-subtitle-action-load.ndjson

## Reproduction Steps
1. Open a video player page with subtitles.
2. Select subtitle text and choose lookup or sentence analysis.
3. Observe that the action stays loading or no result is shown.

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | The subtitle selection click path never reaches the action handler. | Medium | Low | Rejected |
| B | Frontend validation rejects the derived lookup or sentence payload. | Medium | Low | Rejected |
| C | The API request fails, is aborted, or returns a non-2xx response. | High | Low | Confirmed |
| D | A player rerender unmounts the action component before its result can render. | Low | Medium | Rejected |

## Log Evidence
- Log lines 1 and 12: both actions reached their handlers.
- Log lines 4-10: lookup received an empty 502 proxy response and failed JSON parsing.
- Log line 14: sentence analysis received HTTP 502.
- Direct probes: the configured backend had no listener; the Vite `/api` proxy returned 502.

## Verification Conclusion
The backend is stopped. All lookup and analysis requests fail at the Vite proxy before reaching application handlers. The initial mount/unmount events are React development mode behavior and not the cause.
