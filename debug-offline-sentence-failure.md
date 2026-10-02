# Debug Session: offline-sentence-failure
- **Status**: [OPEN]
- **Issue**: Offline word and phrase lookup is slow; sentence analysis and translation both fail.
- **Debug Server**: http://10.4.17.20:7777/event
- **Log File**: `.dbg/trae-debug-log-offline-sentence-failure.ndjson`

## Reproduction Steps
1. Install the current debug APK and enable offline mode with the verified local model.
2. Open a book and query one word or phrase; observe completion latency.
3. Select one English sentence and open sentence analysis.
4. Observe that both translation and grammar analysis show failure.
5. Exit offline mode so locally queued diagnostics can upload.

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | Strict chunk type or source-alignment validation rejects generated JSON | High | Low | **Confirmed** |
| B | The 1024-token sentence response is truncated or times out | High | Low | Rejected by existing logs |
| C | LiteRT callback `Message.toString()` contains a non-text wrapper | Medium | Low | Rejected by existing logs |
| D | Lookup latency is dominated by engine cold start and excessive output budget | High | Low | **Confirmed: repeated 11.8-14.6s GPU init** |
| E | Concurrent sentence consumers cache cancellation or failure | Medium | Medium | Rejected by shared deterministic failure |

## Log Evidence
Existing server logs only cover phase-one validation: GPU initialization about 13.3 seconds and first token about 1.24 seconds. No phase-two generation diagnostics are currently emitted.

The old app uploaded enough phase-two evidence after offline mode was disabled:
- Session `71fa4fc4-bdc3-48d4-a48b-c0e8211dbd38` failed twice with
  `java.io.IOException: llm sentence chunks do not match source` at
  `LlmResponseParser.parseSentenceAnalysis`.
- Both translation and analysis consumed the same failed combined result, explaining the paired UI failure.
- GPU initialization took 14,611 ms and 11,786 ms.
- `TRIM_MEMORY_UI_HIDDEN` level 20 was followed by `backend_closed`, proving the engine was
  incorrectly released for a normal background transition.

Instrumentation added for pre-fix run:
- D: engine warm/cold state, prompt sizes, token budget, first-token and total latency.
- A/B/C: response length and JSON boundary shape, plus sanitized generation/parse errors.
- E: lookup and sentence cache hit and cached success state.

The app cannot POST directly to the debug server while `OFFLINE` is active because doing so
would violate the product's zero-network invariant. These events use the existing local pending
log and upload only after the user exits offline mode.

## Verification Conclusion
Root cause fix implemented:
- Preserve the original source while accepting omitted inter-chunk whitespace, equivalent quote/dash
  typography, and trailing punctuation. Changed words still fail alignment.
- Do not close the engine for `TRIM_MEMORY_UI_HIDDEN`; only critical/complete pressure releases it.
- Expand offline output budgets after comparing server limits: word and phrase combined responses
  use 4,096 tokens, sentence translation plus analysis uses 8,192 tokens, contextual input allows
  8,000 characters, and the aggregate response guard allows 65,536 characters. The measured
  latency fix still comes from preserving the initialized engine; these are safety ceilings.
- MTP was not enabled because the pinned `litertlm-android:0.16.1` Kotlin `EngineConfig` does not
  expose the current documentation's speculative-decoding flag.

Automated verification: 115 unit tests passed and debug APK assembled. Post-fix device verification
is pending.

Second device run evidence, session `1c39bba9-428a-4947-b571-f3ccefa04bfc`:
- Engine reuse is fixed: three lookup requests reported `engineWarm=true`; first-token latency was
  787-802 ms without another initialization.
- Warm lookup completion still took 6,569-6,849 ms for 337-349 response characters, so remaining
  latency is decode volume/speed.
- Sentence generation completed valid JSON in 24,483 ms (695 characters), but the top-level
  `chunks` array was absent (`本地模型 chunks 无效` at `OfflineLlmClient.kt:238`).

Second fix:
- Use compact JSON schemas for lookup and sentence output to reduce generated characters.
- Accept verbose and compact schemas plus `analysis`/`grammar` nesting.
- Map unknown chunk types through the shared whitelist parser.
- If chunks are absent or cannot align, preserve the valid translation and return an exact-source
  single-chunk grammar fallback instead of failing both UI sections.
- Port the normal server's tolerant chunk-alignment behavior into the shared Android parser:
  locate chunks after the current cursor while ignoring whitespace, ASCII case and common English
  typography differences; skip hallucinated chunks; insert `other/补齐` chunks for omitted source
  spans; and force the final output to reproduce the original sentence exactly. This applies to
  standalone and offline modes.

Third device run evidence, session `ec1f584e-bbaa-434c-a5af-4e017bb8f6ef`:
- Sentence parsing succeeded with three chunks.
- Cold sentence request took 23,708 ms: GPU initialization took 12,870 ms, first output arrived at
  14,682 ms, and generation completed at 23,697 ms with 805 response characters. The 8,192-token
  setting was only a ceiling and was not exhausted.
- Warm compact lookup requests improved to 2,927-5,333 ms.
- User reported that the Chinese-meaning UI contains non-Chinese text. Existing diagnostics prove
  one meaning was parsed but do not identify its script.

Current hypotheses:
- F1: the local model emits English in compact `m` despite the Chinese prompt.
- F2: a schema deviation causes English `d` data to be interpreted as `m`.
- F3: the UI reads definitions instead of meanings.
- F4: word and phrase prompts differ in language compliance.

Instrumentation added: only Han/Latin character counts for parsed definitions, meanings and
sentence translation. No selected text, context, model response or path is reported.

Engine lifecycle audit:
- Confirmed manual mode entry and persisted OFFLINE app startup did not warm the shared engine;
  the first lookup or sentence paid the 12-15 second initialization cost.
- All business generation already routes through the single `OfflineEngineSession`.
- Ordinary Activity pause/stop and moderate trim level 20 do not close the engine.
- Engine close remains limited to leaving OFFLINE, critical/complete memory pressure, and
  unrecoverable engine errors.

Lifecycle fix:
- Warm the shared engine before a manual OFFLINE transition completes; roll network policy back if
  warmup fails.
- A proposed automatic process-start warmup was removed after device evidence showed that exposing
  OFFLINE while warmup was still completing could trigger a second native GPU initialization.

Device-freeze evidence, session `876d5f12-de4d-4222-8de8-0a3d1624279a`:
- Manual mode entry started GPU initialization and completed it in 13,147 ms.
- Four milliseconds later, the first lookup reported `engineWarm=false` and started another GPU
  initialization. That second initialization never completed; the next log belongs to a rebooted
  process.
- The rebooted process initialized once successfully in 13,424 ms, proving the model and GPU were
  still usable.

Stability fix:
- Do not expose OFFLINE mode until manual warmup has fully returned.
- Do not proactively initialize on process startup; after process death, the first request performs
  the one serialized lazy initialization.
- Keep pre-existing close rules for leaving OFFLINE, critical memory pressure and fatal engine
  errors.

Language evidence, session `5ea6d59b-1e8e-462d-83b4-74d6f420354c`:
- Engine warmed once in 10,063 ms before OFFLINE was enabled. Sentence, word and phrase requests all
  reported `engineWarm=true`; no duplicate initialization occurred.
- Two word meanings had `meaningHanChars=0` with 101 and 68 Latin characters.
- One phrase meaning had `meaningHanChars=0` with 12 Latin characters.
- Sentence translation also had `translationHanChars=0` and 174 Latin characters.
- UI mapping was previously verified to read `MeaningData.meanings`; the model itself violated the
  requested output language.

Language fix:
- Strengthen first-pass prompts with explicit per-field language rules and concrete Chinese JSON
  examples.
- Validate that every Chinese lookup meaning and its context explanation contains Han characters.
- Validate that sentence translation contains Han characters.
- Only when validation fails, run a compact local Chinese-field repair request. Preserve English
  definitions and original sentence chunks. If repair still has no Chinese, fail explicitly instead
  of displaying English under a Chinese label.

Latest failure evidence and revised root cause:
- Session `1c39bba9-428a-4947-b571-f3ccefa04bfc` generated complete JSON
  (`startsWithBrace=true`, `endsWithBrace=true`) but sentence parsing failed with
  `本地模型 chunks 无效`.
- The translation consumer immediately hit `debug_offline_sentence_cache_hit success=false` and
  reported the identical chunks error. Therefore translation did not independently fail; it reused
  the failed combined translation-and-grammar result.
- Hypothesis A is confirmed: grammar/chunk validation can fail the combined result.
- Hypothesis B is rejected for this reproduction: output was structurally complete, not truncated.
- Hypothesis C is rejected: callback text produced parseable JSON boundaries.
- Hypothesis D is confirmed at the application layer: both consumers share one result/cache.
- Hypothesis E is confirmed by design and runtime cache evidence: the cached grammar failure is
  propagated to translation.

Planned minimal correction:
- Give translation and grammar analysis separate prompts, generation calls, parsers, result caches,
  and failure states.
- Give English definition, Chinese meaning, and contextual explanation the same independent task
  boundaries so one malformed field cannot invalidate unrelated content.
- Keep the single shared engine session; requests remain serialized at the engine boundary to avoid
  duplicate GPU initialization.

Latest reproduction, session `0fa9ecd7-4eb5-44ff-9dc0-44a27b95e7eb`:
- The combined sentence response contained a usable Chinese translation, but embedded unescaped
  quotation marks and malformed nested chunk arrays.
- Generation reported `endsWithBrace=false`; JSON parsing failed at character 22.
- Translation immediately reused `debug_offline_sentence_cache_hit success=false` and displayed the
  same grammar JSON error. This directly confirms cross-task failure propagation.

Independent-task fix:
- `prepareLookup` now only validates and stores lookup context; it performs no model generation.
- English definition, Chinese meaning, contextual explanation, sentence translation, and sentence
  grammar each use one dedicated prompt, parser, mutex, result cache, and failure boundary.
- Sentence translation uses plain text instead of JSON, so quotation marks in natural language
  cannot corrupt a JSON envelope.
- Only successful results are cached. A malformed or transient model reply can be retried.
- The sentence screen starts translation before grammar analysis; the lookup popup starts Chinese
  meaning before English definition and contextual explanation.
- All generation still uses the one serialized `OfflineEngineSession`.
- Expected WorkManager cancellation now rethrows `CancellationException` instead of recording a
  false `standalone sync crashed` error and requesting retry.

Automated verification:
- 123 Android unit tests passed.
- Debug APK assembled successfully.
- APK SHA-256: `e1ea3d9f6ec7db9d3b19818b90cdc07a4811f829d4c04d83517483572310068e`.
- Post-fix device verification is pending; debug instrumentation remains enabled.

Lookup display follow-up:
- Device screenshots showed copied schema placeholders (`词性 中文释义`) for `keypad` and the
  source phrase repeated as a gray part-of-speech label for `make head or tail of them`.
- Root cause: offline prompts used ambiguous positional arrays with literal placeholder examples,
  while normal and standalone modes use named object fields.
- Chinese rows now display only the meaning in every mode. Offline prompts use the shared
  `meanings[].meaning` object shape, omit part of speech, and use concrete examples.
- Offline parsing clears Chinese part-of-speech values, rejects copied schema labels, and removes an
  exact target echo at the start of a Chinese meaning.
- Runtime logs also showed valid JSON for `thingy` and `sequins` rejected because the model used the
  normal-mode nested `meanings[].definitions[]` shape. Offline lookup now accepts both that shape
  and the standalone flat `definitions[]` shape.
- 126 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `85ac5b4041c5d40a090eda7caf674ab20d06d932c06199bb4e61fe3cf15271b1`.

Compact Chinese meaning follow-up:
- Session `882147c7-face-4488-bee3-c75e13a2e4f2` reproduced failures for the phrase
  `make head or tail of them` and words `magnetic`, `sequins`, and `identical`.
- Every generation completed with a valid JSON object, but replies were only 17-25 characters and
  parsing failed with `llm returned no meanings`. The small model simplified the requested
  `meanings` array to a singular `meaning` value.
- Chinese meaning prompts now request the minimal `{"meaning":"..."}` contract.
- The offline adapter accepts singular `meaning`, compact `m`, string arrays, object values, the
  previous pair arrays, and the normal/standalone object arrays, then normalizes all variants to
  `MeaningData`.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `32983f4a766377af3f578972b57c83fbe8a35d3213142afbafecb858ca5c89e9`.

Plain-text lookup protocol:
- Offline English definition, Chinese meaning, contextual explanation, and sentence translation now
  request and consume plain text. Grammar analysis is the only offline task that still requires JSON.
- English definition results omit phonetic and part-of-speech data. Chinese meaning results omit
  part-of-speech data. Existing domain models are populated with empty optional metadata so the
  normal UI path remains unchanged.
- Lookup parsing still removes an optional output label and an echoed target, then validates the
  expected language before exposing the result.
- The previous offline lookup JSON shape normalization was removed; malformed grammar JSON remains
  isolated from the four plain-text tasks.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `9b10bc1745270f9e9ca947183b49a9ad5827a876f430f65e3a5e1752137ae83c`.

Lexical Chinese meaning correction:
- Normal mode queries ECDICT by the word alone and only falls back to an LLM with `单词：<word>`.
  Standalone mode also sends only the word or phrase, without reading context.
- Offline Chinese meaning now follows the same boundary: its model request contains only
  `单词：<word>` or `短语：<phrase>`. Reading context remains exclusive to contextual explanation.
- The prompt asks for the term's own core meanings ordered by modern usage frequency. Multiple
  meanings are separated with Chinese semicolons; part of speech remains omitted in offline mode.
- Offline Chinese meaning has no result cache. Every query or manual refresh runs a new generation,
  while the shared model engine remains warm.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `2468f2b474d886335dacf659a2927cae27b1451bb685f808ac5f8a283c0d2598`.

All offline task result caches removed:
- English definition, Chinese meaning, contextual explanation, sentence translation, and grammar
  analysis no longer retain successful or failed model results.
- Every invocation and manual refresh starts a new model generation. Per-task mutexes remain only
  for concurrency control, and the shared warm engine remains only to avoid reloading the model.
- This supersedes the earlier independent-result-cache design documented above.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `71081fa1a6e2bf87dd3ebe135cf9bf9560d740626ab12ea222959e591195f386`.

Lexical English definition correction:
- Offline English definition now matches standalone mode's task boundary and receives only
  `Word: <word>`; paragraph context is not included.
- Its prompt requests the word's own common English definitions ordered by modern usage frequency.
  Reading context remains exclusive to contextual explanation.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `dacb27e4b5fa6baead36039c2a62e76112238a46109b8017306609d0260ed543`.

Context explanation prompt alignment:
- Offline context explanation now uses the same system prompt and user-message structure as
  standalone mode: `目标单词：<text>` plus the `【】`-highlighted context.
- The prompt explicitly requests sentence role, fixed expressions or modifier structure, contextual
  implications, and prohibits translating the whole paragraph.
- LiteRT thinking is disabled both through SDK configuration and the `/no_think` user-message
  suffix to avoid model-side reasoning latency.
- 127 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `b58949a250cb2068b4703b3701f51ac9cd45cdb5c3cd8e90410a31ea1a523f54`.

Grammar Chinese-key compatibility and synthetic chunk display:
- Session `0dfb7a7a-8fc5-4997-86cd-d068844f5df5` failed
  `Maybe not, but it definitely got an eyeful of the chatterbox.` with
  `llm sentence chunks are invalid`.
- Nearby raw error evidence shows the local model emits chunk objects with Chinese field names such
  as `原文片段`, `类型`, `中文语法作用`, and `中文片段翻译`. Offline normalization now maps these fields and
  Chinese type labels to the shared `SentenceChunk` contract.
- The grammar prompt now uses the standalone-style standard object contract without a literal JSON
  value example. Copied structure placeholders such as `中文结构概括` are discarded.
- Alignment chunks with role `补齐` remain internally for exact source coverage and color positioning,
  but are filtered from the user-visible grammar cards.
- 129 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `70c69402b11c5e907eecc39e7a87504b7c79d0516464cf51b9cef68ae05ff478`.

Grammar malformed JSON, hidden source text, and English runaway output:
- Session `60320a87-58e4-4f5d-8f88-afc10d40d54d` returned a grammar object with a missing
  comma before `"type"` for the `As he was still a sophomore...` sentence. The parser now repairs
  this observed field-boundary error and retries the same grammar task once with a stricter JSON
  prompt when normalization still fails.
- Synthetic alignment chunks containing only punctuation remain hidden. Synthetic chunks containing
  source words are now shown with an empty role, so hiding the `补齐` label cannot hide the sentence.
- The `against` English-definition request generated 4,607 characters and exhausted 1,024 output
  tokens before validation rejected it. The English task now uses an English-only dictionary prompt
  and a 256-token output limit to prevent runaway repetition.
- 133 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `2340507a7496d6f5c82007889381363b839bd0596509d0cb72306bff03b809f3`.

Grammar translation language validation:
- The `Tomohiko got goosebumps...` response populated each chunk's `translation` with the English
  `text` verbatim. The standard-object prompt had stopped explicitly requiring Chinese values.
- Both grammar prompts now require Simplified Chinese for `structure`, `role`, and `translation`,
  and explicitly prohibit copying English text into `translation`.
- Parsed multi-word model chunks without any Han characters in `translation` are rejected and use
  the existing one-time grammar retry. Single-word proper-name chunks remain allowed.
- 134 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `4bdfc0d344121864bb53b6713ca48a7f051a29ecf76169c2fda5d9800c41587d`.

Automatic network-driven offline mode:
- Registered one process-wide `ConnectivityManager.NetworkCallback`; capability changes are
  debounced for one second before checking `NET_CAPABILITY_INTERNET` and `VALIDATED`.
- A lost validated network automatically initializes and enables offline mode. A restored validated
  network closes the local engine, removes the network block, and restores the previous NORMAL or
  STANDALONE mode without invoking model initialization.
- The previous online mode is persisted so restoration still works after a process restart.
- Removed the bookshelf offline switch and model-picker interaction. The row now only shows
  initializing, ready, disabling, failed, or online/disabled status; book navigation remains
  independent.
- 134 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `751a6040631367f5b648533593719b8577a4454735bd3d416885c30fcc0fde94`.

Online bookshelf visibility:
- The offline status row is now rendered only while initialization is running, offline mode is
  ready, or offline initialization has failed.
- IDLE and DISABLING states render no row, title, message, or placeholder, so the original
  bookshelf layout is restored as soon as validated internet connectivity returns.
- 135 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `d55a530404520a5c9a7ea1d74fd3008c750f386435e815b734d300003851b294`.

Online banner hard guard:
- Added a second visibility guard inside the offline status component itself. IDLE and DISABLING
  cannot render the row even if a caller invokes the component.
- Confirmed the rebuilt APK contains neither `当前网络可用` nor `离线模式已关闭`.
- 135 Android unit tests passed and the debug APK assembled successfully.
- APK SHA-256: `0cafaf465f34da65dcfc0d4e299789f11dd566fb609ebfae231f47d85730eca9`.
