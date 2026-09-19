package com.example.orange.ui.wordlist

import android.os.SystemClock
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.example.orange.R
import com.example.orange.data.logging.AppLog
import com.example.orange.data.word.ConfusableWord
import com.example.orange.data.word.WordApi
import com.example.orange.data.word.WordAssociationItem
import java.util.Locale
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin

private const val ASSOCIATION_HINT_LIMIT = 200
private val confusableWordPattern = Regex("^[a-zA-Z][a-zA-Z'-]*$")

internal fun normalizeConfusableQuery(raw: String): String? {
    val normalized = raw.trim().lowercase(Locale.ROOT)
    return normalized.takeIf { confusableWordPattern.matches(it) }
}

internal fun associationHintLength(value: String): Int =
    value.codePointCount(0, value.length)

internal fun acceptsAssociationHint(value: String): Boolean =
    associationHintLength(value) <= ASSOCIATION_HINT_LIMIT

internal fun confusableGroupWordSet(items: List<ConfusableWord>): Set<String> =
    items.mapNotNullTo(linkedSetOf()) { normalizeConfusableQuery(it.word) }

internal fun associationAlreadyInGroup(
    item: WordAssociationItem,
    groupItems: List<ConfusableWord>,
): Boolean {
    val normalized = normalizeConfusableQuery(item.word) ?: return false
    return normalized in confusableGroupWordSet(groupItems)
}

private sealed interface ConfusableLoadState {
    data object Loading : ConfusableLoadState
    data class Success(val items: List<ConfusableWord>) : ConfusableLoadState
    data class Failure(val message: String) : ConfusableLoadState
}

private sealed interface AssociationLoadState {
    data object Idle : AssociationLoadState
    data object Loading : AssociationLoadState
    data class Success(val items: List<WordAssociationItem>) : AssociationLoadState
    data class Failure(val message: String) : AssociationLoadState
}

private data class AssociationRequest(
    val word: String,
    val meaningHint: String,
    val spellingHint: String,
)

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun ConfusableWordsScreen(
    initialWord: String,
    queryInput: String,
    onQueryInputChange: (String) -> Unit,
    onQuerySubmitted: (String) -> Unit,
    onBack: () -> Unit,
    onWordClick: (ConfusableWord) -> Unit,
    onAssociatedWordClick: (WordAssociationItem) -> Unit,
    modifier: Modifier = Modifier,
) {
    val initialQuery = remember(initialWord) { normalizeConfusableQuery(initialWord) }
    val submittedWord = initialQuery
    var requestTick by remember(initialWord) { mutableIntStateOf(0) }
    var validationError by remember(initialWord) { mutableStateOf(initialQuery == null) }
    var groupState by remember(initialWord) {
        mutableStateOf<ConfusableLoadState>(ConfusableLoadState.Loading)
    }
    var meaningHint by remember(initialWord) { mutableStateOf("") }
    var spellingHint by remember(initialWord) { mutableStateOf("") }
    var associationRequest by remember(initialWord) { mutableStateOf<AssociationRequest?>(null) }
    var associationTick by remember(initialWord) { mutableIntStateOf(0) }
    var associationState by remember(initialWord) {
        mutableStateOf<AssociationLoadState>(AssociationLoadState.Idle)
    }
    var addingWord by remember(initialWord) { mutableStateOf<String?>(null) }
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val groupItems = (groupState as? ConfusableLoadState.Success)?.items

    fun submitSearch() {
        val normalized = normalizeConfusableQuery(queryInput)
        if (normalized == null) {
            validationError = true
            return
        }
        validationError = false
        if (submittedWord == normalized) {
            requestTick++
        } else {
            onQuerySubmitted(normalized)
        }
    }

    fun submitAssociation() {
        val word = submittedWord ?: return
        associationRequest = AssociationRequest(
            word = word,
            meaningHint = meaningHint,
            spellingHint = spellingHint,
        )
        associationTick++
    }

    fun addAssociation(item: WordAssociationItem) {
        val word = submittedWord ?: return
        val candidate = normalizeConfusableQuery(item.word) ?: return
        if (addingWord != null) return
        val started = SystemClock.elapsedRealtime()
        // #region debug-point A,B,C,D:add-confusable
        AppLog.event(
            tag = "ConfusableAdd",
            message = "[DEBUG] add started",
            data = org.json.JSONObject()
                .put("sessionId", "confusable-add-stall")
                .put("runId", "pre-fix")
                .put("hypothesisId", "A,B,C,D")
                .put("word", word)
                .put("candidate", candidate),
        )
        // #endregion
        addingWord = candidate
        scope.launch {
            val result = WordApi.addConfusableWord(word, candidate)
            var updatedGroup = result.getOrNull()
            if (updatedGroup == null) {
                for (attempt in 0 until 3) {
                    delay(if (attempt == 0) 300L else 800L)
                    val refreshed = WordApi.confusableWords(word).getOrNull()
                    if (refreshed?.members?.any { it.word.equals(candidate, ignoreCase = true) } == true) {
                        updatedGroup = refreshed
                        break
                    }
                }
            }
            addingWord = null
            // #region debug-point A,B,C,D:add-confusable-result
            AppLog.event(
                tag = "ConfusableAdd",
                message = "[DEBUG] add completed",
                data = org.json.JSONObject()
                    .put("sessionId", "confusable-add-stall")
                    .put("runId", "pre-fix")
                    .put("hypothesisId", "A,B,C,D")
                    .put("word", word)
                    .put("candidate", candidate)
                    .put("durationMs", SystemClock.elapsedRealtime() - started)
                    .put("success", updatedGroup != null)
                    .put("error", result.exceptionOrNull()?.message.orEmpty()),
            )
            // #endregion
            if (updatedGroup != null) {
                groupState = ConfusableLoadState.Success(updatedGroup.members)
            } else {
                snackbarHostState.showSnackbar("加入失败，请重试")
            }
        }
    }

    LaunchedEffect(submittedWord, requestTick) {
        val query = submittedWord ?: return@LaunchedEffect
        groupState = ConfusableLoadState.Loading
        groupState = WordApi.confusableWords(query).fold(
            onSuccess = { ConfusableLoadState.Success(it?.members.orEmpty()) },
            onFailure = { ConfusableLoadState.Failure(it.message ?: "加载失败") },
        )
    }

    LaunchedEffect(associationTick) {
        if (associationTick == 0) return@LaunchedEffect
        val request = associationRequest ?: return@LaunchedEffect
        associationState = AssociationLoadState.Loading
        associationState = WordApi.associateWords(
            word = request.word,
            meaningHint = request.meaningHint,
            spellingHint = request.spellingHint,
        ).fold(
            onSuccess = { AssociationLoadState.Success(it.items) },
            onFailure = { AssociationLoadState.Failure(it.message ?: "联想失败") },
        )
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = { Text("易混词") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            painter = painterResource(R.drawable.ic_back),
                            contentDescription = "返回",
                        )
                    }
                },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding),
        ) {
            OutlinedTextField(
                value = queryInput,
                onValueChange = {
                    onQueryInputChange(it)
                    validationError = false
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                singleLine = true,
                label = { Text("英文单词") },
                isError = validationError,
                supportingText = {
                    if (validationError) {
                        Text("请输入有效的英文单词")
                    }
                },
                trailingIcon = {
                    IconButton(onClick = ::submitSearch) {
                        Icon(
                            painter = painterResource(R.drawable.ic_search),
                            contentDescription = "搜索",
                        )
                    }
                },
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { submitSearch() }),
            )
            if (!validationError && submittedWord != null) {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f),
                    contentPadding = PaddingValues(bottom = 24.dp),
                ) {
                    when (val state = groupState) {
                        ConfusableLoadState.Loading -> item(key = "group-loading") {
                            GroupStatusBox { CircularProgressIndicator() }
                        }
                        is ConfusableLoadState.Failure -> item(key = "group-error") {
                            GroupStatusBox {
                                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                    Text("加载失败", color = MaterialTheme.colorScheme.error)
                                    TextButton(onClick = { requestTick++ }) { Text("重试") }
                                }
                            }
                        }
                        is ConfusableLoadState.Success -> {
                            if (state.items.isEmpty()) {
                                item(key = "group-empty") {
                                    GroupStatusBox {
                                        Text(
                                            "暂无易混词",
                                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        )
                                    }
                                }
                            } else {
                                itemsIndexed(
                                    items = state.items,
                                    key = { _, item -> "group-${item.itemId}" },
                                ) { index, item ->
                                    ConfusableWordRow(
                                        word = item.word,
                                        meaning = item.topMeaningText,
                                        onClick = { onWordClick(item) },
                                    )
                                    if (index < state.items.lastIndex) {
                                        HorizontalDivider(
                                            modifier = Modifier.padding(horizontal = 16.dp),
                                            thickness = 0.5.dp,
                                        )
                                    }
                                }
                            }
                        }
                    }
                    item(key = "association-divider") {
                        HorizontalDivider(
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 20.dp),
                            thickness = 1.dp,
                        )
                    }
                    item(key = "association-radar") {
                        Box(
                            modifier = Modifier.fillMaxWidth(),
                            contentAlignment = Alignment.Center,
                        ) {
                            AssociationRadar(
                                word = submittedWord,
                                loading = associationState == AssociationLoadState.Loading,
                                onAssociate = ::submitAssociation,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .sizeIn(maxWidth = 280.dp)
                                    .aspectRatio(1f),
                            )
                        }
                    }
                    item(key = "association-hints") {
                        AssociationHints(
                            meaningHint = meaningHint,
                            spellingHint = spellingHint,
                            onMeaningHintChange = {
                                if (acceptsAssociationHint(it)) meaningHint = it
                            },
                            onSpellingHintChange = {
                                if (acceptsAssociationHint(it)) spellingHint = it
                            },
                        )
                    }
                    when (val state = associationState) {
                        AssociationLoadState.Idle,
                        AssociationLoadState.Loading,
                        -> Unit
                        is AssociationLoadState.Failure -> item(key = "association-error") {
                            AssociationSectionTitle()
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(16.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                            ) {
                                Text("联想失败", color = MaterialTheme.colorScheme.error)
                                TextButton(onClick = ::submitAssociation) { Text("重试") }
                            }
                        }
                        is AssociationLoadState.Success -> {
                            item(key = "association-title") { AssociationSectionTitle() }
                            if (state.items.isEmpty()) {
                                item(key = "association-empty") {
                                    Text(
                                        text = "暂无联想结果",
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(24.dp),
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                            } else {
                                itemsIndexed(
                                    items = state.items,
                                    key = { index, item -> "association-$index-${item.word}" },
                                ) { _, item ->
                                    AssociationResultRow(
                                        item = item,
                                        onClick = { onAssociatedWordClick(item) },
                                        showAdd = groupItems?.let {
                                            !associationAlreadyInGroup(item, it)
                                        } == true,
                                        adding = addingWord == normalizeConfusableQuery(item.word),
                                        addEnabled = addingWord == null,
                                        onAdd = { addAssociation(item) },
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun GroupStatusBox(content: @Composable () -> Unit) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(112.dp),
        contentAlignment = Alignment.Center,
    ) {
        content()
    }
}

@Composable
private fun AssociationRadar(
    word: String,
    loading: Boolean,
    onAssociate: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Box(modifier = modifier, contentAlignment = Alignment.Center) {
        RadarCanvas(loading = loading, modifier = Modifier.fillMaxSize())
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                text = word,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Button(onClick = onAssociate, enabled = !loading) {
                Text(if (loading) "联想中" else "联想")
            }
        }
    }
}

@Composable
private fun RadarCanvas(
    loading: Boolean,
    modifier: Modifier = Modifier,
) {
    val transition = rememberInfiniteTransition(label = "association-radar")
    val rotation by transition.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(
            animation = tween(1_500, easing = LinearEasing),
        ),
        label = "association-radar-rotation",
    )
    val pulse by transition.animateFloat(
        initialValue = 0.92f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(850, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "association-radar-pulse",
    )
    val radarColor = MaterialTheme.colorScheme.primary
    val guideColor = MaterialTheme.colorScheme.outlineVariant

    Canvas(modifier) {
        val radius = size.minDimension * 0.44f
        if (loading) {
            rotate(degrees = rotation, pivot = center) {
                drawCircle(
                    brush = Brush.sweepGradient(
                        0f to radarColor.copy(alpha = 0f),
                        0.78f to radarColor.copy(alpha = 0f),
                        0.84f to radarColor.copy(alpha = 0.03f),
                        0.90f to radarColor.copy(alpha = 0.09f),
                        0.95f to radarColor.copy(alpha = 0.20f),
                        1f to radarColor.copy(alpha = 0.38f),
                        center = center,
                    ),
                    radius = radius,
                )
            }
        }
        for (scale in listOf(1f, 0.68f, 0.36f)) {
            drawCircle(
                color = guideColor,
                radius = radius * scale,
                style = Stroke(width = 1.dp.toPx()),
            )
        }
        drawLine(
            color = guideColor,
            start = Offset(center.x - radius, center.y),
            end = Offset(center.x + radius, center.y),
            strokeWidth = 1.dp.toPx(),
        )
        drawLine(
            color = guideColor,
            start = Offset(center.x, center.y - radius),
            end = Offset(center.x, center.y + radius),
            strokeWidth = 1.dp.toPx(),
        )
        listOf(
            Offset(center.x - radius * 0.55f, center.y - radius * 0.28f),
            Offset(center.x + radius * 0.48f, center.y - radius * 0.52f),
            Offset(center.x + radius * 0.62f, center.y + radius * 0.31f),
        ).forEach {
            drawCircle(color = radarColor.copy(alpha = 0.65f), radius = 3.dp.toPx(), center = it)
        }
        if (loading) {
            val radians = rotation / 180f * PI.toFloat()
            val end = Offset(
                x = center.x + cos(radians) * radius,
                y = center.y + sin(radians) * radius,
            )
            drawLine(
                color = radarColor,
                start = center,
                end = end,
                strokeWidth = 2.dp.toPx(),
                cap = StrokeCap.Round,
            )
            drawCircle(
                color = radarColor.copy(alpha = 0.5f),
                radius = radius * pulse,
                style = Stroke(width = 2.dp.toPx()),
            )
        }
    }
}

@Composable
private fun AssociationHints(
    meaningHint: String,
    spellingHint: String,
    onMeaningHintChange: (String) -> Unit,
    onSpellingHintChange: (String) -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = "线索",
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.Bold,
        )
        OutlinedTextField(
            value = meaningHint,
            onValueChange = onMeaningHintChange,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("意思特征（可选）") },
            minLines = 2,
            maxLines = 3,
            supportingText = {
                Text("${associationHintLength(meaningHint)}/$ASSOCIATION_HINT_LIMIT")
            },
        )
        OutlinedTextField(
            value = spellingHint,
            onValueChange = onSpellingHintChange,
            modifier = Modifier.fillMaxWidth(),
            label = { Text("拼写特征（可选）") },
            minLines = 2,
            maxLines = 3,
            supportingText = {
                Text("${associationHintLength(spellingHint)}/$ASSOCIATION_HINT_LIMIT")
            },
        )
    }
}

@Composable
private fun AssociationSectionTitle() {
    Text(
        text = "联想结果",
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        style = MaterialTheme.typography.titleSmall,
        fontWeight = FontWeight.Bold,
    )
}

@Composable
private fun AssociationResultRow(
    item: WordAssociationItem,
    onClick: () -> Unit,
    showAdd: Boolean,
    adding: Boolean,
    addEnabled: Boolean,
    onAdd: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp),
        shape = RoundedCornerShape(6.dp),
        tonalElevation = 1.dp,
    ) {
        ConfusableWordRow(
            word = item.word,
            meaning = item.topMeaningText,
            onClick = onClick,
            trailingContent = if (showAdd) {
                {
                    if (adding) {
                        Box(
                            modifier = Modifier.size(48.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(20.dp),
                                strokeWidth = 2.dp,
                            )
                        }
                    } else {
                        IconButton(
                            onClick = onAdd,
                            enabled = addEnabled,
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_add),
                                contentDescription = "加入易混词组",
                            )
                        }
                    }
                }
            } else {
                null
            },
        )
    }
}

@Composable
private fun ConfusableWordRow(
    word: String,
    meaning: String,
    onClick: () -> Unit,
    trailingContent: (@Composable () -> Unit)? = null,
) {
    val linkBlue = if (isSystemInDarkTheme()) Color(0xFF90CAF9) else Color(0xFF1565C0)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = word,
            modifier = Modifier
                .weight(0.42f)
                .clickable(onClick = onClick)
                .padding(vertical = 4.dp),
            color = linkBlue,
            fontWeight = FontWeight.Bold,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = meaning.ifBlank { "暂无释义" },
            modifier = Modifier
                .weight(0.58f)
                .padding(start = 16.dp),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        trailingContent?.invoke()
    }
}
