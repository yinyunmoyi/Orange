package com.example.orange.ui.learning

import android.media.AudioAttributes
import android.media.MediaPlayer
import android.os.SystemClock
import com.example.orange.data.logging.AppLog as Log
import androidx.activity.compose.BackHandler
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.audiosync.AudioCacheRepository
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.audiosync.CachedAudioPlayer
import com.example.orange.data.learning.LearningApi
import com.example.orange.data.learning.LearningCard
import com.example.orange.data.learning.LearningCurrent
import com.example.orange.data.learning.LEARNING_ITEM_PHRASE
import com.example.orange.data.learning.LEARNING_ITEM_WORD
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.WordApi
import com.example.orange.ui.components.WordLevelBadge
import com.example.orange.ui.components.WordLevelBadgeSize
import com.example.orange.ui.components.WordMetaChips
import com.example.orange.ui.wordlist.ConfusableWordsScreen
import com.example.orange.ui.wordlist.WordDetailScreen
import com.example.orange.ui.wordlist.buildHighlightedSentence
import kotlinx.coroutines.async
import kotlinx.coroutines.launch
import org.json.JSONObject
import java.util.UUID
import kotlin.math.abs
import kotlin.math.ceil

private data class WordDetailTarget(
    val itemId: Long,
    val word: String,
)
private fun reportLearningTransition(
    location: String,
    message: String,
    data: JSONObject = JSONObject(),
    traceId: String? = null,
) {
    LearningApi.reportEvent(location, message, data, traceId)
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun WordLearningScreen(
    sessionId: Long,
    onBack: () -> Unit,
    onPlayContextVideo: (ContextVideo) -> Unit = {},
    modifier: Modifier = Modifier,
) {
    var current by remember(sessionId) { mutableStateOf<LearningCurrent?>(null) }
    var loading by remember { mutableStateOf(true) }
    var submitting by remember { mutableStateOf(false) }
    var revealed by remember { mutableStateOf(false) }
    var revealEnabled by remember { mutableStateOf(false) }
    var masteryArmed by remember { mutableStateOf(false) }
    var audioRegenerationState by remember(sessionId) {
        mutableStateOf(AudioRegenerationState.Closed)
    }
    var error by remember { mutableStateOf<String?>(null) }
    var reload by remember { mutableStateOf(0) }
    var confusableInitialWord by remember(sessionId) { mutableStateOf<String?>(null) }
    var confusableQueryInput by remember(sessionId) { mutableStateOf("") }
    var selectedConfusableWord by remember(sessionId) { mutableStateOf<WordDetailTarget?>(null) }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = remember { SnackbarHostState() }
    val debugScreenId = remember(sessionId) {
        "learning-$sessionId-${SystemClock.elapsedRealtime()}"
    }
    val debugMountedAt = remember(sessionId) { SystemClock.elapsedRealtime() }
    val cardOffset = remember { Animatable(0f) }
    val countdownProgress = remember { Animatable(1f) }
    val exitDistance = with(LocalDensity.current) { 520.dp.toPx() }
    val player = remember {
        MediaPlayer().apply {
            setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_MEDIA)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                    .build()
            )
        }
    }
    DisposableEffect(Unit) {
        onDispose { runCatching { player.release() } }
    }
    suspend fun playTrackedAudio(card: LearningCard, rawUrl: String, mode: String) {
        CachedAudioPlayer.play(
            context,
            player,
            WordApi.audioAbsoluteUrl(rawUrl),
            "WordLearningAudio",
        )
        WordApi.recordFavoriteAction(
            itemType = if (card.itemType == LEARNING_ITEM_PHRASE) "phrase" else "word",
            itemId = card.itemId,
            eventId = UUID.randomUUID().toString(),
            action = "audio_played",
            source = "android_learning",
            metadata = JSONObject().put("mode", mode),
        )
    }
    DisposableEffect(debugScreenId) {
        reportLearningTransition(
            location = "WordLearningScreen:lifecycle",
            message = "learning screen mounted",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("sessionId", sessionId)
                .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
        )
        onDispose {
            reportLearningTransition(
                location = "WordLearningScreen:lifecycle",
                message = "learning screen disposed",
                data = JSONObject()
                    .put("screenId", debugScreenId)
                    .put("sessionId", sessionId)
                    .put("lifetimeMs", SystemClock.elapsedRealtime() - debugMountedAt)
                    .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
            )
        }
    }
    val unsupportedCard = current?.card?.let {
        it.itemType != LEARNING_ITEM_WORD && it.itemType != LEARNING_ITEM_PHRASE
    } == true
    val regeneratingAudio = audioRegenerationState == AudioRegenerationState.Regenerating
    val interactionLocked = submitting || regeneratingAudio

    LaunchedEffect(sessionId, reload) {
        val debugLoadStarted = SystemClock.elapsedRealtime()
        reportLearningTransition(
            location = "WordLearningScreen:currentCard",
            message = "current card load started",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("sessionId", sessionId)
                .put("reload", reload)
                .put("elapsedRealtimeMs", debugLoadStarted),
        )
        loading = true
        error = null
        val debugLoadResult = LearningApi.currentCard(sessionId)
        reportLearningTransition(
            location = "WordLearningScreen:currentCard",
            message = "current card load completed",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("sessionId", sessionId)
                .put("durationMs", SystemClock.elapsedRealtime() - debugLoadStarted)
                .put("success", debugLoadResult.isSuccess)
                .put("queueItemId", debugLoadResult.getOrNull()?.queueItemId ?: 0L)
                .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
        )
        debugLoadResult.fold(
            onSuccess = { current = it },
            onFailure = {
                Log.e("WordLearning", "load current failed session=$sessionId", it)
                error = it.message ?: "加载失败"
            },
        )
        loading = false
    }
    LaunchedEffect(current?.queueItemId, current?.turnNo) {
        current?.card?.let { card ->
            card.audioUrl.takeIf { it.isNotBlank() }?.let {
                playTrackedAudio(card, it, "auto")
            }
        }
    }
    LaunchedEffect(current?.queueItemId, current?.turnNo) {
        revealed = false
        revealEnabled = false
        masteryArmed = false
        audioRegenerationState = reduceAudioRegenerationState(
            audioRegenerationState,
            AudioRegenerationEvent.ItemChanged,
        ).state
        countdownProgress.snapTo(1f)
        countdownProgress.animateTo(
            targetValue = 0f,
            animationSpec = tween(durationMillis = 3_000, easing = LinearEasing),
        )
        revealEnabled = true
    }

    fun submitCurrent(
        action: String,
        direction: Float,
        keepMasteryOnFailure: Boolean,
        request: suspend (LearningCurrent, String) -> Result<LearningCurrent>,
    ) {
        val snapshot = current ?: return
        if (interactionLocked || snapshot.queueItemId <= 0) return
        val debugSubmitStarted = SystemClock.elapsedRealtime()
        val debugTraceId = "$action-${snapshot.queueItemId}-$debugSubmitStarted"
        reportLearningTransition(
            location = "WordLearningScreen:submitCurrent",
            message = "answer transition started",
            data = JSONObject()
                .put("action", action)
                .put("queueItemId", snapshot.queueItemId)
                .put("turnNo", snapshot.turnNo)
                .put("elapsedRealtimeMs", debugSubmitStarted),
            traceId = debugTraceId,
        )
        submitting = true
        error = null
        scope.launch {
            val debugRequestStarted = SystemClock.elapsedRealtime()
            val requestDeferred = async { request(snapshot, debugTraceId) }
            cardOffset.animateTo(direction * exitDistance, tween(durationMillis = 220))
            reportLearningTransition(
                location = "WordLearningScreen:submitCurrent",
                message = "exit animation completed while answer request was running",
                data = JSONObject()
                    .put("elapsedMs", debugRequestStarted - debugSubmitStarted)
                    .put("elapsedRealtimeMs", debugRequestStarted),
                traceId = debugTraceId,
            )
            val result = requestDeferred.await()
            val debugRequestFinished = SystemClock.elapsedRealtime()
            reportLearningTransition(
                location = "WordLearningScreen:submitCurrent",
                message = "answer request completed",
                data = JSONObject()
                    .put("requestDurationMs", debugRequestFinished - debugRequestStarted)
                    .put("elapsedMs", debugRequestFinished - debugSubmitStarted)
                    .put("success", result.isSuccess)
                    .put("nextQueueItemId", result.getOrNull()?.queueItemId ?: 0L)
                    .put("elapsedRealtimeMs", debugRequestFinished),
                traceId = debugTraceId,
            )
            val next = result.getOrNull()
            if (next != null) {
                current = next
                revealed = false
                cardOffset.snapTo(-direction * exitDistance)
                cardOffset.animateTo(0f, tween(durationMillis = 260))
            } else {
                val cause = result.exceptionOrNull()
                Log.e(
                    "WordLearning",
                    "$action failed session=$sessionId queue=${snapshot.queueItemId} turn=${snapshot.turnNo}",
                    cause,
                )
                error = cause?.message ?: "提交失败"
                if (!keepMasteryOnFailure) {
                    masteryArmed = false
                }
                cardOffset.animateTo(0f, tween(durationMillis = 180))
            }
            submitting = false
            reportLearningTransition(
                location = "WordLearningScreen:submitCurrent",
                message = "answer transition completed",
                data = JSONObject()
                    .put("totalDurationMs", SystemClock.elapsedRealtime() - debugSubmitStarted)
                    .put("success", next != null)
                    .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
                traceId = debugTraceId,
            )
        }
    }

    fun answer(value: String, direction: Float) {
        masteryArmed = false
        audioRegenerationState = reduceAudioRegenerationState(
            audioRegenerationState,
            AudioRegenerationEvent.ItemChanged,
        ).state
        submitCurrent("answer-$value", direction, keepMasteryOnFailure = false) { snapshot, traceId ->
            LearningApi.answer(
                sessionId = sessionId,
                queueItemId = snapshot.queueItemId,
                turnNo = snapshot.turnNo,
                answer = value,
                traceId = traceId,
            )
        }
    }

    fun masteryClick() {
        if (interactionLocked || current?.card == null) return
        audioRegenerationState = reduceAudioRegenerationState(
            audioRegenerationState,
            AudioRegenerationEvent.ItemChanged,
        ).state
        if (!masteryArmed) {
            masteryArmed = true
            return
        }
        submitCurrent("mastery", 1f, keepMasteryOnFailure = true) { snapshot, traceId ->
            LearningApi.masterCurrentItem(
                sessionId = sessionId,
                queueItemId = snapshot.queueItemId,
                turnNo = snapshot.turnNo,
                traceId = traceId,
            )
        }
    }

    fun audioRegenerationClick() {
        val snapshot = current?.card ?: return
        if (submitting || regeneratingAudio) return
        masteryArmed = false
        val transition = reduceAudioRegenerationState(
            audioRegenerationState,
            AudioRegenerationEvent.Click,
        )
        audioRegenerationState = transition.state
        if (!transition.regenerate) return

        error = null
        scope.launch {
            LearningApi.regenerateAudio(snapshot.itemType, snapshot.itemId).fold(
                onSuccess = { audioUrl ->
                    val itemType = when (snapshot.itemType) {
                        LEARNING_ITEM_WORD -> "word"
                        LEARNING_ITEM_PHRASE -> "phrase"
                        else -> return@fold
                    }
                    AudioCacheRepository.invalidate(context, itemType, snapshot.itemId)
                    AudioCacheRepository.cacheRegenerated(
                        context,
                        itemType,
                        snapshot.itemId,
                        audioUrl,
                    )
                    current = current?.let { learningCurrent ->
                        val card = learningCurrent.card
                        if (card?.itemType == snapshot.itemType && card.itemId == snapshot.itemId) {
                            learningCurrent.copy(card = card.copy(audioUrl = audioUrl))
                        } else {
                            learningCurrent
                        }
                    }
                    audioRegenerationState = reduceAudioRegenerationState(
                        audioRegenerationState,
                        AudioRegenerationEvent.Succeeded,
                    ).state
                    playTrackedAudio(snapshot, audioUrl, "regenerated")
                    AudioSyncScheduler.enqueue(context)
                    snackbarHostState.showSnackbar("音频已更新")
                },
                onFailure = { cause ->
                    Log.e(
                        "WordLearning",
                        "regenerate audio failed itemType=${snapshot.itemType} itemId=${snapshot.itemId}",
                        cause,
                    )
                    error = cause.message ?: "音频重新生成失败"
                    audioRegenerationState = reduceAudioRegenerationState(
                        audioRegenerationState,
                        AudioRegenerationEvent.Failed,
                    ).state
                },
            )
        }
    }

    confusableInitialWord?.let { initialWord ->
        val closeConfusables = {
            confusableInitialWord = null
            confusableQueryInput = ""
            selectedConfusableWord = null
        }
        BackHandler(onBack = closeConfusables)
        Box(modifier = modifier.fillMaxSize()) {
            ConfusableWordsScreen(
                initialWord = initialWord,
                queryInput = confusableQueryInput,
                onQueryInputChange = { confusableQueryInput = it },
                onQuerySubmitted = { confusableInitialWord = it },
                onBack = closeConfusables,
                onWordClick = {
                    selectedConfusableWord = WordDetailTarget(it.itemId, it.word)
                },
                onAssociatedWordClick = {
                    selectedConfusableWord = WordDetailTarget(0L, it.word)
                },
                modifier = Modifier.fillMaxSize(),
            )
            selectedConfusableWord?.let { selected ->
                val closeDetail = { selectedConfusableWord = null }
                BackHandler(onBack = closeDetail)
                WordDetailScreen(
                    itemId = selected.itemId,
                    word = selected.word,
                    onBack = closeDetail,
                    onPlayContextVideo = onPlayContextVideo,
                    modifier = Modifier.fillMaxSize(),
                )
            }
        }
        return
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = { Text("学习") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(painterResource(R.drawable.ic_back), contentDescription = "返回")
                    }
                },
                actions = {
                    current?.takeIf { !it.completed && it.card != null && !unsupportedCard }?.let {
                        val card = it.card ?: return@let
                        if (card.itemType == LEARNING_ITEM_WORD) {
                            IconButton(
                                onClick = {
                                    selectedConfusableWord = null
                                    confusableInitialWord = card.word
                                    confusableQueryInput = card.word
                                },
                                enabled = !interactionLocked,
                            ) {
                                Icon(
                                    painter = painterResource(R.drawable.ic_confusable_twins),
                                    contentDescription = "查看易混词",
                                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                        AudioRegenerationAction(
                            state = audioRegenerationState,
                            enabled = !submitting,
                            onClick = ::audioRegenerationClick,
                        )
                        MasteryActionIcon(
                            armed = masteryArmed,
                            enabled = !interactionLocked,
                            onClick = ::masteryClick,
                        )
                    }
                },
            )
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) {
            when {
                loading -> CircularProgressIndicator()
                current?.completed == true -> Completion(current!!.progress, onBack)
                unsupportedCard -> Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    val card = current?.card
                    Text("不支持的学习内容", color = MaterialTheme.colorScheme.error)
                    Text(
                        "itemType=${card?.itemType} itemId=${card?.itemId}",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    TextButton(onClick = { reload++ }) { Text("重试") }
                }
                current?.card != null -> {
                    LearningCardContent(
                        card = current!!.card!!,
                        progress = current!!.progress,
                        queueType = current!!.queueType,
                        revealed = revealed,
                        revealEnabled = revealEnabled,
                        countdownProgress = countdownProgress.value,
                        submitting = interactionLocked,
                        cardOffsetX = cardOffset.value,
                        exitDistance = exitDistance,
                        error = error,
                        onCardClick = {
                            if (interactionLocked) return@LearningCardContent
                            val card = current?.card
                            card?.audioUrl?.takeIf { it.isNotBlank() }?.let {
                                scope.launch {
                                    playTrackedAudio(card, it, "reveal")
                                }
                            }
                            if (!revealEnabled) {
                                scope.launch { countdownProgress.snapTo(0f) }
                                revealEnabled = true
                            }
                            revealed = true
                        },
                        onPlay = {
                            if (!interactionLocked) {
                                scope.launch {
                                    current?.card?.let { card ->
                                        playTrackedAudio(card, it, "manual")
                                    }
                                }
                            }
                        },
                        onPlayContextVideo = { video ->
                            reportLearningTransition(
                                location = "WordLearningScreen:onPlayContextVideo",
                                message = "context video requested",
                                data = JSONObject()
                                    .put("screenId", debugScreenId)
                                    .put("videoId", video.id)
                                    .put("queueItemId", current?.queueItemId ?: 0L)
                                    .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
                            )
                            onPlayContextVideo(video)
                        },
                        onUnknown = { answer("unknown", -1f) },
                        onKnown = { answer("known", 1f) },
                    )
                }
                else -> Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(error ?: "没有可学习内容", color = MaterialTheme.colorScheme.error)
                    TextButton(onClick = { reload++ }) { Text("重试") }
                }
            }
        }
    }
}

@Composable
private fun MasteryActionIcon(
    armed: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val activeColor = Color(0xFFFFB300)
    val iconColor by animateColorAsState(
        targetValue = if (armed) activeColor else MaterialTheme.colorScheme.onSurfaceVariant,
        animationSpec = tween(durationMillis = 180),
        label = "mastery-icon-color",
    )
    Box(modifier = Modifier.size(56.dp), contentAlignment = Alignment.Center) {
        if (armed) {
            MasteryGlow(activeColor)
        }
        IconButton(
            onClick = onClick,
            enabled = enabled,
            modifier = Modifier.size(48.dp),
        ) {
            Icon(
                painter = painterResource(R.drawable.ic_mastery_crown),
                contentDescription = if (armed) "再次点击设为已掌握" else "设为已掌握",
                tint = if (enabled) iconColor else iconColor.copy(alpha = 0.55f),
            )
        }
    }
}

@Composable
private fun MasteryGlow(color: Color) {
    val transition = rememberInfiniteTransition(label = "mastery-glow")
    val pulse by transition.animateFloat(
        initialValue = 0.7f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 1_000, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "mastery-glow-pulse",
    )
    Canvas(Modifier.fillMaxSize()) {
        val radius = size.minDimension * 0.5f * pulse
        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(
                    color.copy(alpha = 0.42f),
                    color.copy(alpha = 0.16f),
                    Color.Transparent,
                ),
                center = center,
                radius = radius,
            ),
            radius = radius,
        )
    }
}

@Composable
private fun LearningCardContent(
    card: LearningCard,
    progress: com.example.orange.data.learning.LearningSessionProgress,
    queueType: String,
    revealed: Boolean,
    revealEnabled: Boolean,
    countdownProgress: Float,
    submitting: Boolean,
    cardOffsetX: Float,
    exitDistance: Float,
    error: String?,
    onCardClick: () -> Unit,
    onPlay: (String) -> Unit,
    onPlayContextVideo: (ContextVideo) -> Unit,
    onUnknown: () -> Unit,
    onKnown: () -> Unit,
) {
    Column(Modifier.fillMaxSize().padding(20.dp)) {
        LearningSessionProgressBar(progress)
        Spacer(Modifier.height(8.dp))
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f)
                .graphicsLayer {
                    translationX = cardOffsetX
                    rotationZ = if (exitDistance == 0f) 0f else cardOffsetX / exitDistance * 8f
                    alpha = 1f - (abs(cardOffsetX) / exitDistance).coerceIn(0f, 1f) * 0.2f
                },
        ) {
            LearningCardHeader(
                card = card,
                queueType = queueType,
                submitting = submitting,
                onPlay = onPlay,
            )
            Surface(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
                    .clickable(enabled = !submitting, onClick = onCardClick),
                shape = RoundedCornerShape(16.dp),
                color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = if (revealed) 1f else 0.25f),
            ) {
                if (revealed) {
                    AnswerContent(
                        card = card,
                        onPlay = onPlay,
                        onPlayContextVideo = onPlayContextVideo,
                        playbackEnabled = !submitting,
                    )
                } else {
                    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        if (!revealEnabled) {
                            CountdownClock(countdownProgress)
                        } else {
                            Text("点击查看释义", color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
        if (error != null) {
            Text(error, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 8.dp))
        }
        Spacer(Modifier.height(12.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedButton(onClick = onUnknown, enabled = !submitting, modifier = Modifier.weight(1f)) {
                Text("不认识")
            }
            Button(onClick = onKnown, enabled = !submitting, modifier = Modifier.weight(1f)) {
                Text("认识")
            }
        }
    }
}

@Composable
private fun LearningCardHeader(
    card: LearningCard,
    queueType: String,
    submitting: Boolean,
    onPlay: (String) -> Unit,
) {
    val wordStyle = MaterialTheme.typography.displaySmall.copy(
        fontSize = 36.sp,
        fontWeight = FontWeight.Bold,
    )
    val phoneticStyle = MaterialTheme.typography.bodyLarge
    val textMeasurer = rememberTextMeasurer()
    val density = LocalDensity.current
    val hasPhonetic = card.itemType == LEARNING_ITEM_WORD && card.phonetic.isNotBlank()
    val hasAudio = card.audioUrl.isNotBlank()
    val hasLevel = card.itemType == LEARNING_ITEM_WORD && card.level > 0
    val isNew = queueType == "new"

    BoxWithConstraints(modifier = Modifier.fillMaxWidth()) {
        val requiredWidthPx = with(density) {
            val wordWidth = textMeasurer.measure(
                text = card.word,
                style = wordStyle,
                maxLines = 1,
                softWrap = false,
            ).size.width
            val newIconWidth = if (isNew) 19.dp.roundToPx() else 0
            val levelWidth = if (hasLevel) {
                8.dp.roundToPx() + WordLevelBadgeSize.roundToPx()
            } else {
                0
            }
            val phoneticWidth = if (hasPhonetic) {
                8.dp.roundToPx() + textMeasurer.measure(
                    text = card.phonetic,
                    style = phoneticStyle,
                    maxLines = 1,
                    softWrap = false,
                ).size.width
            } else {
                0
            }
            val audioWidth = if (hasAudio) 48.dp.roundToPx() else 0
            wordWidth + newIconWidth + levelWidth + phoneticWidth + audioWidth
        }
        val useSingleLine = requiredWidthPx <= with(density) { maxWidth.roundToPx() }

        if (useSingleLine) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                WordTitle(
                    card = card,
                    isNew = isNew,
                    style = wordStyle,
                    maxLines = 1,
                )
                if (hasPhonetic) {
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = card.phonetic,
                        style = phoneticStyle,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                    )
                }
                if (hasAudio) {
                    PronunciationButton(
                        audioUrl = card.audioUrl,
                        enabled = !submitting,
                        onPlay = onPlay,
                    )
                }
            }
        } else {
            Column(modifier = Modifier.fillMaxWidth()) {
                WordTitle(
                    card = card,
                    isNew = isNew,
                    style = wordStyle,
                    maxLines = 2,
                    modifier = Modifier.fillMaxWidth(),
                )
                if (hasPhonetic || hasAudio) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        if (hasPhonetic) {
                            Text(
                                text = card.phonetic,
                                style = phoneticStyle,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 2,
                                modifier = Modifier.weight(1f),
                            )
                        } else {
                            Spacer(Modifier.weight(1f))
                        }
                        if (hasAudio) {
                            PronunciationButton(
                                audioUrl = card.audioUrl,
                                enabled = !submitting,
                                onPlay = onPlay,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun WordTitle(
    card: LearningCard,
    isNew: Boolean,
    style: androidx.compose.ui.text.TextStyle,
    maxLines: Int,
    modifier: Modifier = Modifier,
) {
    var wordFontSize by remember(card.word, maxLines) { mutableStateOf(style.fontSize) }
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.Top,
    ) {
        Text(
            text = card.word,
            style = style,
            fontSize = wordFontSize,
            maxLines = maxLines,
            modifier = if (maxLines > 1) Modifier.weight(1f, fill = false) else Modifier,
            onTextLayout = { result ->
                if (result.hasVisualOverflow && wordFontSize.value > 18f) {
                    wordFontSize = (wordFontSize.value - 2f).sp
                }
            },
        )
        if (isNew) {
            Icon(
                painter = painterResource(R.drawable.ic_new_word_sparkle),
                contentDescription = "新学",
                tint = Color.Unspecified,
                modifier = Modifier
                    .offset(x = 1.dp, y = (-3).dp)
                    .size(18.dp),
            )
        }
        if (card.itemType == LEARNING_ITEM_WORD && card.level > 0) {
            Spacer(Modifier.width(8.dp))
            WordLevelBadge(card.level)
        }
    }
}

@Composable
private fun PronunciationButton(
    audioUrl: String,
    enabled: Boolean,
    onPlay: (String) -> Unit,
) {
    IconButton(
        onClick = { onPlay(audioUrl) },
        enabled = enabled,
    ) {
        Icon(
            painter = painterResource(R.drawable.ic_volume),
            contentDescription = "播放发音",
        )
    }
}

@Composable
private fun CountdownClock(progress: Float) {
    val trackColor = MaterialTheme.colorScheme.outlineVariant
    val progressColor = MaterialTheme.colorScheme.primary
    val seconds = ceil(progress.coerceIn(0f, 1f) * 3f).toInt().coerceAtLeast(1)
    Box(modifier = Modifier.size(76.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            val strokeWidth = 6.dp.toPx()
            drawCircle(
                color = trackColor,
                radius = (size.minDimension - strokeWidth) / 2,
                center = Offset(size.width / 2, size.height / 2),
                style = Stroke(width = strokeWidth),
            )
            drawArc(
                color = progressColor,
                startAngle = -90f,
                sweepAngle = progress.coerceIn(0f, 1f) * 360f,
                useCenter = false,
                style = Stroke(width = strokeWidth, cap = StrokeCap.Round),
            )
        }
        Text(
            text = seconds.toString(),
            style = MaterialTheme.typography.headlineMedium,
            color = progressColor,
        )
    }
}

@Composable
private fun AnswerContent(
    card: LearningCard,
    onPlay: (String) -> Unit,
    onPlayContextVideo: (ContextVideo) -> Unit,
    playbackEnabled: Boolean,
) {
    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (
            card.itemType == LEARNING_ITEM_WORD &&
            (card.oxford == 1 || card.tags.isNotEmpty())
        ) {
            WordMetaChips(
                pos = card.pos,
                oxford = card.oxford,
                tags = card.tags,
                showPos = false,
            )
        }
        Section("中文释义")
        card.chinese.forEach { Text("${it.pos}  ${it.text}".trim()) }
        Section("语境")
        card.contexts.forEach { context ->
            Surface(shape = RoundedCornerShape(8.dp), color = MaterialTheme.colorScheme.surface) {
                Column(Modifier.padding(10.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                buildHighlightedSentence(
                                    context.sentence,
                                    context.highlightStart,
                                    context.highlightEnd,
                                    SpanStyle(
                                        color = MaterialTheme.colorScheme.primary,
                                        fontWeight = FontWeight.Bold,
                                    ),
                                ),
                            )
                            if (context.translation.isNotBlank()) {
                                Spacer(Modifier.height(4.dp))
                                Text(context.translation, fontSize = 13.sp)
                            }
                        }
                        if (context.audioUrl.isNotBlank() || context.videos.isNotEmpty()) {
                            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                if (context.audioUrl.isNotBlank()) {
                                    IconButton(
                                        onClick = { onPlay(context.audioUrl) },
                                        enabled = playbackEnabled,
                                        modifier = Modifier.size(36.dp),
                                    ) {
                                        Icon(painterResource(R.drawable.ic_volume), contentDescription = "播放语境")
                                    }
                                }
                                context.videos.forEachIndexed { index, video ->
                                    IconButton(
                                        onClick = { onPlayContextVideo(video) },
                                        enabled = playbackEnabled,
                                        modifier = Modifier.size(36.dp),
                                    ) {
                                        Icon(
                                            painterResource(R.drawable.ic_video_play),
                                            contentDescription = if (context.videos.size == 1) {
                                                "播放视频语境"
                                            } else {
                                                "播放视频语境 ${index + 1}"
                                            },
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        if (card.note.isNotBlank()) {
            Section("备注")
            Text(
                text = card.note,
                color = MaterialTheme.colorScheme.onSurface,
                fontSize = 14.sp,
                lineHeight = 21.sp,
            )
        }
        if (card.itemType == LEARNING_ITEM_WORD) {
            Section("英文释义")
            card.english.forEach { meaning ->
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.Top,
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    if (meaning.pos.isNotBlank()) {
                        Surface(
                            shape = RoundedCornerShape(6.dp),
                            color = MaterialTheme.colorScheme.secondaryContainer,
                        ) {
                            Text(
                                text = meaning.pos,
                                color = MaterialTheme.colorScheme.onSecondaryContainer,
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 12.sp,
                                modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                            )
                        }
                    }
                    Text(
                        text = meaning.text,
                        color = MaterialTheme.colorScheme.onSurface,
                        fontSize = 14.sp,
                        lineHeight = 21.sp,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
        }
    }
}

@Composable
private fun Section(text: String) {
    Text(text, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary)
}

@Composable
private fun Completion(
    progress: com.example.orange.data.learning.LearningSessionProgress,
    onBack: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(20.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        LearningSessionProgressBar(progress)
        Spacer(Modifier.weight(1f))
        Text("今日学习完成", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(20.dp))
        Button(onClick = onBack) { Text("结束学习") }
        Spacer(Modifier.weight(1f))
    }
}
