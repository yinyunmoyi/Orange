# Debug Session: playback-controls-early
- **Status**: [OPEN]
- **Issue**: Video context playback controls appear when buffering finishes; expected controls hidden until playback ends.
- **Debug Server**: Pending startup
- **Log File**: `.dbg/trae-debug-log-playback-controls-early.ndjson`

## Reproduction Steps
1. Open a word detail page containing a video context.
2. Tap the video-context play button.
3. Wait for buffering to complete.
4. Observe playback controls appearing before playback ends.

## Hypotheses & Verification
| ID | Hypothesis | Evidence |
|----|------------|----------|
| A | `showPlaybackControls` remains true from a previous ended playback | Pending |
| B | PlayerView composition/update auto-shows its controller | Pending |
| C | Media3 briefly emits `STATE_ENDED` before `STATE_READY` | Pending |
| D | A touch/controller event shows controls independently of Compose state | Pending |
| E | Installed APK does not match current source | Pending |

## Log Evidence
- Current build marker `controls-debug-v2` is present on the emulator.
- For both 8.5-second video ID 1 and 1.89-second video ID 3, playback states were strictly BUFFERING (2) -> READY (3) -> ENDED (4).
- At READY + 100 ms, `showPlaybackControls=false`, `useController=false`, and `controllerVisible=false`.
- Controls were enabled only after ENDED at position 1984 ms for video ID 3.

## Verification Conclusion
- A and C are rejected by state logs.
- E is rejected for the connected emulator, although no physical device is connected for comparison.
- B/D are not reproduced on the emulator, but the implementation only enforced visibility during Compose updates. The fix will also enforce controller visibility directly from Media3 callbacks so device-specific internal controller/touch timing cannot expose controls during playback.

Post-fix verification with the 1.89-second video ID 3:
- BUFFERING and READY both report `showPlaybackControls=false`, `useController=false`, and `controllerVisible=false`.
- The playback screenshot at 0.9 seconds shows only video and subtitles.
- ENDED occurs at 1989 ms; only then are controls enabled and displayed.
- `assembleDebug` and `testDebugUnitTest` both completed successfully.
