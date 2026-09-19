package com.example.orange.ui.reader

import android.graphics.Rect
import android.media.AudioAttributes
import android.media.MediaPlayer
import com.example.orange.data.logging.AppLog as Log
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.audiosync.CachedAudioPlayer
import com.example.orange.data.word.ChineseMeaning
import com.example.orange.data.word.Definition
import com.example.orange.data.word.ExplainData
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.WordApi
import com.example.orange.ui.components.LoadingDots
import com.example.orange.ui.components.WordMetaChips
import com.example.orange.ui.note.NoteSection
import kotlinx.coroutines.launch
import java.util.UUID

data class LookupUiState(
    val word: String,
    val anchorRectPx: Rect,
    val context: String,
    val wordStart: Int,
    val wordEnd: Int,
    val paragraph: String,
    val selectionStart: Int,
    val selectionEnd: Int,
    val isPhrase: Boolean = false,
)

sealed class LoadState<out T> {
    object Loading : LoadState<Nothing>()
    data class Success<T>(val data: T) : LoadState<T>()
    data class Failure(val message: String) : LoadState<Nothing>()
}

@Composable
fun LookupPopup(
    state: LookupUiState,
    palette: ReaderPalette,
    onDismiss: () -> Unit,
    onEditNote: (itemType: String, text: String, note: String, onSaved: (String) -> Unit) -> Unit,
) {
    val configuration = LocalConfiguration.current
    val context = LocalContext.current
    val density = LocalDensity.current
    val scope = rememberCoroutineScope()

    val screenWpx = with(density) { configuration.screenWidthDp.dp.toPx() }
    val screenHpx = with(density) { configuration.screenHeightDp.dp.toPx() }
    val gapPx = with(density) { 8.dp.toPx() }
    val marginPx = with(density) { 16.dp.toPx() }

    val popupW = screenWpx * 0.75f
    val popupH = screenHpx * 0.35f * 9f / 8f

    val anchor = state.anchorRectPx
    val preferBelowTop = anchor.bottom + gapPx
    val preferAboveTop = anchor.top - gapPx - popupH
    val top: Float = when {
        preferBelowTop + popupH <= screenHpx - marginPx -> preferBelowTop
        preferAboveTop >= marginPx -> preferAboveTop
        else -> {
            val spaceAbove = anchor.top - marginPx
            val spaceBelow = screenHpx - anchor.bottom - marginPx
            if (spaceBelow >= spaceAbove) {
                (anchor.bottom + gapPx).coerceAtMost(screenHpx - popupH - marginPx)
            } else {
                (anchor.top - gapPx - popupH).coerceAtLeast(marginPx)
            }
        }
    }
    val anchorCenterX = (anchor.left + anchor.right) / 2f
    val left: Float = (anchorCenterX - popupW / 2f).coerceIn(marginPx, screenWpx - popupW - marginPx)

    val popupWDp = with(density) { popupW.toDp() }
    val popupHDp = with(density) { popupH.toDp() }

    var refreshTick by remember(state.word) { mutableIntStateOf(0) }
    var english by remember(state.word, refreshTick) { mutableStateOf<LoadState<LookupData>>(LoadState.Loading) }
    var chinese by remember(state.word, refreshTick) { mutableStateOf<LoadState<MeaningData>>(LoadState.Loading) }
    var explain by remember(state.word, refreshTick) { mutableStateOf<LoadState<ExplainData>>(LoadState.Loading) }
    var note by remember(state.word) { mutableStateOf<String?>(null) }
    var noteLoading by remember(state.word) { mutableStateOf(true) }
    var noteError by remember(state.word) { mutableStateOf<String?>(null) }

    var favorited by remember(state.word) { mutableStateOf<Boolean?>(null) }
    var favoritedItemId by remember(state.word) { mutableStateOf<Long?>(null) }
    var favoriting by remember(state.word) { mutableStateOf(false) }
    var initiallyFavorited by remember(state.word) { mutableStateOf<Boolean?>(null) }
    var resetting by remember(state.word) { mutableStateOf(false) }
    val lookupEventId = remember(state.word, state.paragraph, state.selectionStart, state.selectionEnd) {
        UUID.randomUUID().toString()
    }
    val coroutineScope = rememberCoroutineScope()

    LaunchedEffect(state.word, state.paragraph, state.selectionStart, state.selectionEnd) {
        val status = if (state.isPhrase) {
            WordApi.phraseFavoriteStatus(state.word)
        } else {
            WordApi.favoriteStatus(state.word)
        }
        val statusData = status.getOrElse {
            Log.w("LookupPopup", "favoriteStatus failed word=${state.word} err_msg=${it.message}", it)
            favorited = false
            favoritedItemId = null
            if (initiallyFavorited == null) initiallyFavorited = false
            return@LaunchedEffect
        }
        val newItemId = if (statusData.favorited && statusData.itemId > 0L) statusData.itemId else null
        favorited = statusData.favorited
        favoritedItemId = newItemId
        if (initiallyFavorited == null) initiallyFavorited = statusData.favorited
        if (!statusData.favorited || newItemId == null) return@LaunchedEffect
        WordApi.recordFavoriteAction(
            itemType = if (state.isPhrase) "phrase" else "word",
            itemId = newItemId,
            eventId = lookupEventId,
            action = "lookup_opened",
            source = "android_reader",
        ).onFailure {
            Log.w("LookupPopup", "lookup action failed word=${state.word}", it)
        }

        val paragraphValid = state.paragraph.isNotBlank() &&
            state.selectionStart >= 0 &&
            state.selectionEnd > state.selectionStart
        if (!paragraphValid) {
            Log.i(
                "LookupPopup",
                "context task auto skip invalid paragraph mode=append word=${state.word} " +
                    "paragraph_len=${state.paragraph.length} " +
                    "start=${state.selectionStart} end=${state.selectionEnd}",
            )
            return@LaunchedEffect
        }
        val itemType = if (state.isPhrase) "phrase" else "word"
        Log.i(
            "LookupPopup",
            "context task auto start mode=append word=${state.word} item_id=$newItemId",
        )
        WordApi.createContextTask(
            itemType = itemType,
            itemId = newItemId,
            paragraph = state.paragraph,
            selectionStart = state.selectionStart,
            selectionEnd = state.selectionEnd,
        ).onSuccess {
            Log.i(
                "LookupPopup",
                "context task auto ok mode=append word=${state.word} " +
                    "paragraph=${state.paragraph.take(120)}",
            )
        }.onFailure { t ->
            Log.w(
                "LookupPopup",
                "context task auto failed mode=append word=${state.word} " +
                    "paragraph=${state.paragraph.take(120)} err_msg=${t.message}",
                t,
            )
        }
    }

    LaunchedEffect(state.word, state.isPhrase) {
        noteLoading = true
        noteError = null
        WordApi.getNote(if (state.isPhrase) "phrase" else "word", state.word).fold(
            onSuccess = { note = it.note },
            onFailure = {
                Log.w("LookupPopup", "note failed word=${state.word} err_msg=${it.message}", it)
                noteError = "备注加载失败"
            },
        )
        noteLoading = false
    }

    LaunchedEffect(state.word, refreshTick) {
        if (state.isPhrase) {
            english = LoadState.Success(LookupData(state.word, "", emptyList(), emptyList()))
        } else {
            launch {
                english = WordApi.lookup(state.word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("LookupPopup", "lookup failed word=${state.word} err_type=${it.javaClass.name} err_msg=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
        }
        launch {
            chinese = if (state.isPhrase) {
                WordApi.phrase(state.word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("LookupPopup", "phrase failed word=${state.word} err_type=${it.javaClass.name} err_msg=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            } else {
                WordApi.meaning(state.word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("LookupPopup", "meaning failed word=${state.word} err_type=${it.javaClass.name} err_msg=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
        }
        launch {
            explain = WordApi.explain(
                word = state.word,
                context = state.context,
                wordStart = state.wordStart,
                wordEnd = state.wordEnd,
            ).fold(
                onSuccess = { LoadState.Success(it) },
                onFailure = {
                    Log.w("LookupPopup", "explain failed word=${state.word} err_type=${it.javaClass.name} err_msg=${it.message}", it)
                    LoadState.Failure("加载失败")
                },
            )
        }
    }

    val player = remember {
        MediaPlayer().apply {
            setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_MEDIA)
                    .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
                    .build()
            )
        }
    }
    DisposableEffect(Unit) {
        onDispose { runCatching { player.release() } }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .pointerInput(state.word) { detectTapGestures(onTap = { onDismiss() }) },
    ) {
        Card(
            colors = CardDefaults.cardColors(containerColor = palette.bg, contentColor = palette.fg),
            elevation = CardDefaults.cardElevation(defaultElevation = 8.dp),
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier
                .offset { IntOffset(left.toInt(), top.toInt()) }
                .width(popupWDp)
                .height(popupHDp)
                .pointerInput(Unit) { detectTapGestures(onTap = {}) },
        ) {
            Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
                LookupHeader(
                    word = state.word,
                    english = english,
                    isPhrase = state.isPhrase,
                    palette = palette,
                    onPlay = { url ->
                        scope.launch {
                            CachedAudioPlayer.play(context, player, url, "LookupAudio")
                            favoritedItemId?.let { itemId ->
                                WordApi.recordFavoriteAction(
                                    itemType = if (state.isPhrase) "phrase" else "word",
                                    itemId = itemId,
                                    eventId = UUID.randomUUID().toString(),
                                    action = "audio_played",
                                    source = "android_reader",
                                )
                            }
                        }
                    },
                    onRefresh = { refreshTick += 1 },
                    isFavorited = favorited,
                    favoriting = favoriting,
                    favoriteContentReady = if (state.isPhrase) {
                        (chinese as? LoadState.Success)?.data?.meanings?.isNotEmpty() == true
                    } else {
                        english is LoadState.Success || chinese is LoadState.Success
                    },
                    onToggleFavorite = {
                        if (favoriting) return@LookupHeader
                        if (favorited == true) {
                            // 语境追加已在打开弹层时自动发起；已收藏状态下点击五角星不做二次动作
                            return@LookupHeader
                        }
                        val eng = (english as? LoadState.Success)?.data
                        val zh = (chinese as? LoadState.Success)?.data
                        val itemType = if (state.isPhrase) "phrase" else "word"
                        val paragraphValid = state.paragraph.isNotBlank() &&
                            state.selectionStart >= 0 &&
                            state.selectionEnd > state.selectionStart

                        if ((state.isPhrase && zh?.meanings.isNullOrEmpty()) || (!state.isPhrase && eng == null && zh == null)) {
                            Log.i(
                                "LookupPopup",
                                "favorite skip word=${state.word} english=$english chinese=$chinese",
                            )
                            return@LookupHeader
                        }
                        favoriting = true
                        coroutineScope.launch {
                            val favoriteResult = if (state.isPhrase) {
                                WordApi.favoritePhrase(state.word, requireNotNull(zh))
                            } else {
                                WordApi.favorite(state.word, eng, zh)
                            }
                            val favoriteError = favoriteResult.exceptionOrNull()
                            val itemId = favoriteResult.getOrNull()
                            if (favoriteError != null || itemId == null) {
                                favoriting = false
                                Log.w(
                                    "LookupPopup",
                                    "favorite failed word=${state.word} " +
                                        "err_msg=${favoriteError?.message ?: "missing itemId"}",
                                    favoriteError,
                                )
                                return@launch
                            }

                            favorited = true
                            favoritedItemId = itemId
                            favoriting = false
                            Log.i("LookupPopup", "favorite ok word=${state.word} item_id=$itemId")

                            if (!paragraphValid) {
                                Log.w(
                                    "LookupPopup",
                                    "context task skip invalid paragraph mode=first-save word=${state.word} " +
                                        "paragraph_len=${state.paragraph.length} " +
                                        "start=${state.selectionStart} end=${state.selectionEnd}",
                                )
                                return@launch
                            }
                            Log.i(
                                "LookupPopup",
                                "context task start mode=first-save word=${state.word} item_id=$itemId",
                            )
                            WordApi.createContextTask(
                                itemType = itemType,
                                itemId = itemId,
                                paragraph = state.paragraph,
                                selectionStart = state.selectionStart,
                                selectionEnd = state.selectionEnd,
                            ).onSuccess {
                                Log.i(
                                    "LookupPopup",
                                    "context task ok mode=first-save word=${state.word} " +
                                        "paragraph=${state.paragraph.take(120)}",
                                )
                            }.onFailure {
                                Log.w(
                                    "LookupPopup",
                                    "context task failed mode=first-save word=${state.word} " +
                                        "paragraph=${state.paragraph.take(120)} err_msg=${it.message}",
                                    it,
                                )
                            }
                        }
                    },
                    resetting = resetting,
                    showReset = initiallyFavorited == true,
                    onResetClick = onResetClick@{
                        if (resetting) return@onResetClick
                        val itemId = favoritedItemId
                        if (favorited != true || itemId == null) return@onResetClick
                        val itemType = if (state.isPhrase) "phrase" else "word"
                        resetting = true
                        coroutineScope.launch {
                            val result = WordApi.resetFavoriteLearning(itemType, itemId)
                            resetting = false
                            result.onSuccess {
                                Log.i(
                                    "LookupPopup",
                                    "learning reset ok word=${state.word} item_id=$itemId",
                                )
                            }.onFailure { t ->
                                Log.w(
                                    "LookupPopup",
                                    "learning reset failed word=${state.word} item_id=$itemId err_msg=${t.message}",
                                    t,
                                )
                            }
                        }
                    },
                )
                Spacer(Modifier.height(12.dp))
                HorizontalDivider(color = palette.disabledFg.copy(alpha = 0.3f))
                val listState = rememberLazyListState()
                LookupBody(
                    english = english,
                    chinese = chinese,
                    explain = explain,
                    isPhrase = state.isPhrase,
                    palette = palette,
                    listState = listState,
                    note = note,
                    noteLoading = noteLoading,
                    noteError = noteError,
                    onEditNote = {
                        onEditNote(
                            if (state.isPhrase) "phrase" else "word",
                            state.word,
                            note.orEmpty(),
                        ) { saved ->
                            note = saved
                            noteError = null
                        }
                    },
                    modifier = Modifier.fillMaxWidth().weight(1f),
                )
            }
        }
    }
}

@Composable
private fun LookupHeader(
    word: String,
    english: LoadState<LookupData>,
    isPhrase: Boolean,
    palette: ReaderPalette,
    onPlay: (String) -> Unit,
    onRefresh: () -> Unit,
    isFavorited: Boolean?,
    favoriting: Boolean,
    favoriteContentReady: Boolean,
    onToggleFavorite: () -> Unit,
    resetting: Boolean,
    showReset: Boolean,
    onResetClick: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(
                text = word,
                fontWeight = FontWeight.Bold,
                fontSize = 20.sp,
                color = palette.fg,
                modifier = Modifier.weight(1f),
            )
            if (!isPhrase) {
                when (english) {
                    is LoadState.Success -> {
                        val pronunciation = english.data.pronunciations
                            .firstOrNull { it.audioUrl.isNotBlank() }
                        if (pronunciation != null) {
                            IconButton(
                                onClick = {
                                    onPlay(WordApi.audioAbsoluteUrl(pronunciation.audioUrl))
                                },
                            ) {
                                Icon(
                                    painter = painterResource(R.drawable.ic_volume),
                                    contentDescription = "播放发音",
                                    tint = palette.accent,
                                )
                            }
                        }
                    }
                    LoadState.Loading -> LoadingDots(color = palette.fg)
                    is LoadState.Failure -> Unit
                }
            }
            IconButton(
                onClick = onToggleFavorite,
                enabled = !favoriting && isFavorited != null &&
                    (isFavorited == true || favoriteContentReady),
            ) {
                Icon(
                    painter = painterResource(
                        if (isFavorited == true) R.drawable.ic_star else R.drawable.ic_star_outline,
                    ),
                    contentDescription = if (isFavorited == true) "已收藏" else "收藏",
                    tint = palette.accent,
                )
            }
            if (showReset) {
                IconButton(
                    onClick = onResetClick,
                    enabled = !resetting,
                ) {
                    Icon(
                        painter = painterResource(R.drawable.ic_hourglass),
                        contentDescription = "重置学习进度",
                        tint = palette.accent,
                    )
                }
            }
            IconButton(onClick = onRefresh) {
                Icon(
                    painter = painterResource(R.drawable.ic_refresh),
                    contentDescription = "刷新",
                    tint = palette.accent,
                )
            }
        }
        if (!isPhrase && english is LoadState.Success) {
            val meta = english.data
            if (meta.pos.isNotBlank() || meta.oxford == 1 || meta.tags.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                WordMetaChips(
                    pos = meta.pos,
                    oxford = meta.oxford,
                    tags = meta.tags,
                    modifier = Modifier.fillMaxWidth(),
                    labelColor = palette.disabledFg,
                    chipBackground = palette.disabledFg.copy(alpha = 0.15f),
                    posColor = palette.accent,
                    oxfordColor = palette.accent,
                )
            }
        }
    }
}

@Composable
private fun LookupBody(
    english: LoadState<LookupData>,
    chinese: LoadState<MeaningData>,
    explain: LoadState<ExplainData>,
    isPhrase: Boolean,
    palette: ReaderPalette,
    listState: LazyListState,
    note: String?,
    noteLoading: Boolean,
    noteError: String?,
    onEditNote: () -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        state = listState,
        modifier = modifier.drawVerticalScrollbar(listState, palette),
        contentPadding = PaddingValues(vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        item {
            SectionHeader(title = "中文释义", palette = palette)
        }
        renderChinese(chinese, palette)
        item { Spacer(Modifier.height(8.dp)) }
        item {
            SectionHeader(title = "AI 语境解释", palette = palette)
        }
        renderExplain(explain, palette)
        if (!isPhrase) {
            item { Spacer(Modifier.height(8.dp)) }
            item {
                SectionHeader(title = "英文释义", palette = palette)
            }
            renderEnglish(english, palette)
        }
        item { Spacer(Modifier.height(8.dp)) }
        item {
            NoteSection(
                note = note,
                loading = noteLoading,
                error = noteError,
                onEdit = onEditNote,
                titleColor = palette.fg,
                bodyColor = palette.fg,
                mutedColor = palette.disabledFg,
                accentColor = palette.accent,
            )
        }
    }
}

private fun LazyListScope.renderEnglish(
    state: LoadState<LookupData>,
    palette: ReaderPalette,
) {
    when (state) {
        LoadState.Loading -> item { InlineLoading(palette) }
        is LoadState.Failure -> item { InlineFailure(state.message, palette) }
        is LoadState.Success -> {
            if (state.data.definitions.isEmpty()) {
                item { InlineEmpty(palette) }
            } else {
                items(state.data.definitions) { def ->
                    DefinitionRow(pos = def.pos, text = def.text, palette = palette)
                }
            }
        }
    }
}

private fun LazyListScope.renderChinese(
    state: LoadState<MeaningData>,
    palette: ReaderPalette,
) {
    when (state) {
        LoadState.Loading -> item { InlineLoading(palette) }
        is LoadState.Failure -> item { InlineFailure(state.message, palette) }
        is LoadState.Success -> {
            if (state.data.meanings.isEmpty()) {
                item { InlineEmpty(palette) }
            } else {
                items(state.data.meanings) { m ->
                    ChineseRow(m = m, palette = palette)
                }
            }
        }
    }
}

private fun LazyListScope.renderExplain(
    state: LoadState<ExplainData>,
    palette: ReaderPalette,
) {
    when (state) {
        LoadState.Loading -> item { InlineLoading(palette) }
        is LoadState.Failure -> item { InlineFailure(state.message, palette) }
        is LoadState.Success -> {
            val text = state.data.explanation
            if (text.isBlank()) {
                item { InlineEmpty(palette) }
            } else {
                item {
                    Text(
                        text = text,
                        color = palette.fg,
                        fontSize = 14.sp,
                        modifier = Modifier.fillMaxWidth().padding(vertical = 2.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun SectionHeader(title: String, palette: ReaderPalette) {
    Text(
        text = title,
        fontWeight = FontWeight.SemiBold,
        color = palette.fg,
        fontSize = 14.sp,
        modifier = Modifier.padding(bottom = 4.dp),
    )
}

@Composable
private fun DefinitionRow(pos: String, text: String, palette: ReaderPalette) {
    Row(modifier = Modifier.fillMaxWidth()) {
        if (pos.isNotBlank()) {
            Text(
                text = pos,
                color = palette.disabledFg,
                fontSize = 13.sp,
            )
            Spacer(Modifier.width(6.dp))
        }
        Text(
            text = text,
            color = palette.fg,
            fontSize = 14.sp,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun ChineseRow(m: ChineseMeaning, palette: ReaderPalette) {
    Row(modifier = Modifier.fillMaxWidth()) {
        if (m.pos.isNotBlank()) {
            Text(
                text = m.pos,
                color = palette.disabledFg,
                fontSize = 13.sp,
            )
            Spacer(Modifier.width(6.dp))
        }
        Text(
            text = m.meaning,
            color = palette.fg,
            fontSize = 14.sp,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun InlineLoading(palette: ReaderPalette) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
    ) {
        Text(text = "加载中", color = palette.disabledFg, fontSize = 13.sp)
        Spacer(Modifier.width(8.dp))
        LoadingDots(color = palette.fg)
    }
}

@Composable
private fun InlineFailure(message: String, palette: ReaderPalette) {
    Text(text = message, color = palette.disabledFg, fontSize = 13.sp)
}

@Composable
private fun InlineEmpty(palette: ReaderPalette) {
    Text(text = "暂无内容", color = palette.disabledFg, fontSize = 13.sp)
}

private fun Modifier.drawVerticalScrollbar(
    state: LazyListState,
    palette: ReaderPalette,
): Modifier = this.drawWithContent {
    drawContent()
    val layoutInfo = state.layoutInfo
    val visible = layoutInfo.visibleItemsInfo
    val totalItems = layoutInfo.totalItemsCount
    if (totalItems == 0 || visible.isEmpty()) return@drawWithContent

    val viewport = (layoutInfo.viewportEndOffset - layoutInfo.viewportStartOffset).toFloat()
    if (viewport <= 0f) return@drawWithContent

    val avgItemSize = visible.map { it.size }.average().toFloat().coerceAtLeast(1f)
    val estimatedTotal = avgItemSize * totalItems
    if (estimatedTotal <= viewport) return@drawWithContent

    val scrolled = state.firstVisibleItemIndex * avgItemSize + state.firstVisibleItemScrollOffset

    val verticalMargin = 4.dp.toPx()
    val trackHeight = size.height - 2f * verticalMargin
    val minBar = 24.dp.toPx()
    val barHeight = (viewport / estimatedTotal * trackHeight).coerceIn(minBar, trackHeight)
    val maxScroll = (estimatedTotal - viewport).coerceAtLeast(1f)
    val barTop = verticalMargin + (scrolled / maxScroll).coerceIn(0f, 1f) * (trackHeight - barHeight)

    val barWidth = 3.dp.toPx()
    val barX = size.width - barWidth - 2.dp.toPx()
    drawRoundRect(
        color = palette.fg.copy(alpha = 0.6f),
        topLeft = Offset(barX, barTop),
        size = Size(barWidth, barHeight),
        cornerRadius = CornerRadius(barWidth / 2f, barWidth / 2f),
    )
}
