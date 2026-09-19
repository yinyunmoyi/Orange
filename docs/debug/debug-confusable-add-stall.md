# Debug Session: confusable-add-stall
- **Status**: [OPEN]
- **Issue**: 易混词页点击加号添加未收藏候选词时长时间转圈，完成后组列表不刷新；期望改为仅添加易混词组，不自动收藏。
- **Debug Server**: `<debug-server>`
- **Log File**: `.dbg/trae-debug-log-confusable-add-stall.ndjson`

## Reproduction Steps
1. 在易混词页搜索一个单词并生成联想结果。
2. 对一个尚未收藏的候选词点击加号。
3. 观察按钮长时间转圈，易混词组不刷新。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Expected Signal |
|----|------------|------------|--------|-----------------|
| A | 未收藏词同步调用中文释义生成导致长等待 | High | Low | **Confirmed by execution path**：加号同步调用 `EnsureWordFullyFavorited`，其中必须完成 `GetMeaning` 后才能分组 |
| B | 音频下载或 TTS 初始化阻塞 | Medium | Low | **Confirmed as possible contributor**：同一同步初始化路径还包含词典和音频准备 |
| C | 收藏已完成但分组合并失败 | Medium | Low | **Confirmed as design risk**：两个收藏分别提交后才执行独立分组事务 |
| D | 服务端返回错误但客户端只展示通用失败提示 | Medium | Low | **Confirmed**：客户端丢弃具体异常，仅显示“加入失败，请重试” |

## Instrumentation Plan
- 复用现有常驻 Android 网络日志和服务端 `ensure`、`group-add` 分阶段日志。
- 如现有证据不足，再补充仅用于本会话的网络上报点。

## Log Evidence
- 用户现场：未收藏候选词点击后持续转圈，组列表不刷新。
- 当前服务端日志中无对应 `group-add` 记录，现场发生在常驻日志覆盖之前。
- 代码路径证据：`AddWordToSimilarGroup` 先串行调用两次 `EnsureWordFullyFavorited`，后者依次执行词典查询、中文释义、音频准备和收藏写入，最后才执行 `MergeGroup`。

## Verification Conclusion
保留“收藏后才能加入词组”的现有模型。修复内容：
- 初始化单词改为 ECDICT 本地优先，避免每次未收藏词先等待外部词典最多三轮重试。
- 易混词查询和添加使用 NSD 缓存的局域网服务地址，连接失败才回退公网。
- 添加响应失败后重查三次分组，恢复“服务端已成功、客户端响应丢失”的界面状态。
- 客户端等待上限由 180 秒缩短到 45 秒。

待真实设备验证。
