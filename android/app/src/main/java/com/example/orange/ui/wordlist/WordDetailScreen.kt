package com.example.orange.ui.wordlist

import android.media.AudioAttributes
import android.media.MediaPlayer
import com.example.orange.data.logging.AppLog as Log
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.audiosync.CachedAudioPlayer
import com.example.orange.data.word.ChineseMeaning
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.ContextsData
import com.example.orange.data.word.Definition
import com.example.orange.data.word.FavoriteStatus
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.WordApi
import com.example.orange.ui.components.LoadingDots
import com.example.orange.ui.components.WordMetaChips
import com.example.orange.ui.components.WordLevelBadge
import com.example.orange.ui.note.NoteEditScreen
import com.example.orange.ui.note.NoteSection
import kotlinx.coroutines.launch
import java.util.UUID

sealed class LoadState<out T> {
    object Loading : LoadState<Nothing>()
    data class Success<T>(val data: T) : LoadState<T>()
    data class Failure(val message: String) : LoadState<Nothing>()
}

internal fun favoriteItemIdForDelete(status: FavoriteStatus?): Long? =
    status?.itemId?.takeIf { status.favorited && it > 0L }

internal fun favoriteItemIdForReset(status: FavoriteStatus?): Long? =
    status?.itemId?.takeIf { status.favorited && it > 0L }

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun WordDetailScreen(
    itemId: Long,
    word: String,
    onBack: () -> Unit,
    onPlayContextVideo: (ContextVideo) -> Unit = {},
    modifier: Modifier = Modifier,
) {
    var english by remember(word) { mutableStateOf<LoadState<LookupData>>(LoadState.Loading) }
    var chinese by remember(word) { mutableStateOf<LoadState<MeaningData>>(LoadState.Loading) }
    var contexts by remember(word) { mutableStateOf<LoadState<ContextsData>>(LoadState.Loading) }
    var note by remember(word) { mutableStateOf<String?>(null) }
    var noteLoading by remember(word) { mutableStateOf(true) }
    var noteError by remember(word) { mutableStateOf<String?>(null) }
    var editingNote by remember(word) { mutableStateOf(false) }
    var favoriteStatus by remember(word) { mutableStateOf<FavoriteStatus?>(null) }
    var favoriting by remember(word) { mutableStateOf(false) }
    var resetting by remember(word) { mutableStateOf(false) }
    var reloadTick by remember(word) { mutableIntStateOf(0) }
    val detailEventId = remember(itemId, word) { UUID.randomUUID().toString() }
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    LaunchedEffect(word, reloadTick) {
        english = LoadState.Loading
        chinese = LoadState.Loading
        contexts = LoadState.Loading
        noteLoading = true
        noteError = null
        favoriteStatus = null
        kotlinx.coroutines.supervisorScope {
            launch {
                english = WordApi.lookup(word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("WordDetail", "lookup failed word=$word err=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
            launch {
                chinese = WordApi.meaning(word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("WordDetail", "meaning failed word=$word err=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
            launch {
                contexts = WordApi.contexts(word).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.w("WordDetail", "contexts failed word=$word err=${it.message}", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
            launch {
                noteLoading = true
                noteError = null
                WordApi.getNote("word", word).fold(
                    onSuccess = { note = it.note },
                    onFailure = {
                        Log.w("WordDetail", "note failed word=$word err=${it.message}", it)
                        noteError = "备注加载失败"
                    },
                )
                noteLoading = false
            }
            launch {
                WordApi.favoriteStatus(word).fold(
                    onSuccess = {
                        favoriteStatus = it
                        if (it.favorited && it.itemId > 0L) {
                            WordApi.recordFavoriteAction(
                                itemType = "word",
                                itemId = it.itemId,
                                eventId = detailEventId,
                                action = "detail_opened",
                                source = "android_word_detail",
                            ).onFailure { error ->
                                Log.w("WordDetail", "detail action failed itemId=${it.itemId}", error)
                            }
                        }
                    },
                    onFailure = {
                        Log.w("WordDetail", "favorite status failed word=$word err=${it.message}", it)
                        favoriteStatus = null
                    },
                )
            }
        }
    }

    if (editingNote) {
        NoteEditScreen(
            itemType = "word",
            text = word,
            initialNote = note.orEmpty(),
            onBack = { editingNote = false },
            onSaved = {
                note = it
                noteError = null
                editingNote = false
            },
            modifier = modifier,
        )
        return
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

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("单词详情") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            painter = painterResource(R.drawable.ic_back),
                            contentDescription = "返回",
                        )
                    }
                },
                actions = {
                    val hasLoadFailure =
                        english is LoadState.Failure ||
                            chinese is LoadState.Failure ||
                            contexts is LoadState.Failure ||
                            noteError != null
                    if (hasLoadFailure) {
                        IconButton(onClick = { reloadTick++ }) {
                            Icon(
                                painter = painterResource(R.drawable.ic_refresh),
                                contentDescription = "重新加载",
                                tint = MaterialTheme.colorScheme.primary,
                            )
                        }
                    }
                    val isFavorited = favoriteStatus?.favorited == true
                    val favoriteContentReady =
                        english is LoadState.Success || chinese is LoadState.Success
                    IconButton(
                        enabled = !favoriting &&
                            favoriteStatus != null &&
                            (isFavorited || favoriteContentReady),
                        onClick = {
                            if (isFavorited || favoriting) return@IconButton
                            val englishData = (english as? LoadState.Success)?.data
                            val chineseData = (chinese as? LoadState.Success)?.data
                            favoriting = true
                            scope.launch {
                                WordApi.favorite(word, englishData, chineseData).fold(
                                    onSuccess = { favoriteItemId ->
                                        favoriteStatus = FavoriteStatus(
                                            favorited = true,
                                            itemId = favoriteItemId,
                                            level = 1,
                                        )
                                        favoriting = false
                                    },
                                    onFailure = { error ->
                                        favoriting = false
                                        Log.e(
                                            "WordDetail",
                                            "favorite failed word=$word",
                                            error,
                                        )
                                        snackbarHostState.showSnackbar("收藏失败，请重试")
                                    },
                                )
                            }
                        },
                    ) {
                        Icon(
                            painter = painterResource(
                                if (isFavorited) R.drawable.ic_star else R.drawable.ic_star_outline,
                            ),
                            contentDescription = if (isFavorited) "已收藏" else "收藏",
                            tint = MaterialTheme.colorScheme.primary,
                        )
                    }
                    favoriteItemIdForReset(favoriteStatus)?.let { favoriteItemId ->
                        IconButton(
                            onClick = {
                                if (resetting) return@IconButton
                                resetting = true
                                scope.launch {
                                    WordApi.resetFavoriteLearning("word", favoriteItemId).fold(
                                        onSuccess = {
                                            resetting = false
                                            snackbarHostState.showSnackbar("学习进度已重置")
                                        },
                                        onFailure = { error ->
                                            resetting = false
                                            Log.e(
                                                "WordDetail",
                                                "learning reset failed itemId=$favoriteItemId word=$word",
                                                error,
                                            )
                                            snackbarHostState.showSnackbar("重置失败，请重试")
                                        },
                                    )
                                }
                            },
                            enabled = !resetting,
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_hourglass),
                                contentDescription = "重置学习进度",
                                tint = MaterialTheme.colorScheme.primary,
                            )
                        }
                    }
                    favoriteItemIdForDelete(favoriteStatus)?.let { favoriteItemId ->
                        FavoriteDeleteAction(
                            itemKey = "word:$favoriteItemId",
                            deleteFavorite = { WordApi.deleteFavorite("word", favoriteItemId) },
                            onDeleted = onBack,
                            onDeleteFailed = { error ->
                                Log.e(
                                    "WordDetail",
                                    "delete failed itemId=$favoriteItemId word=$word",
                                    error,
                                )
                                scope.launch {
                                    snackbarHostState.showSnackbar("删除失败，请重试")
                                }
                            },
                        )
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { innerPadding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            item {
                WordHeader(
                    word = word,
                    english = english,
                    level = favoriteStatus?.level ?: 0,
                    onPlay = { url ->
                        scope.launch {
                            CachedAudioPlayer.play(context, player, url, "WordDetailAudio")
                            val favoriteItemId = favoriteStatus?.itemId ?: itemId
                            if (favoriteItemId > 0L) {
                                WordApi.recordFavoriteAction(
                                    itemType = "word",
                                    itemId = favoriteItemId,
                                    eventId = UUID.randomUUID().toString(),
                                    action = "audio_played",
                                    source = "android_word_detail",
                                )
                            }
                        }
                    },
                )
            }
            item { Spacer(Modifier.height(12.dp)) }
            item {
                SectionTitle("中文释义")
            }
            renderChinese(chinese)
            item { Spacer(Modifier.height(12.dp)) }
            item {
                SectionTitle("语境")
            }
            renderContexts(
                state = contexts,
                onPlay = { url ->
                    scope.launch {
                        CachedAudioPlayer.play(
                            context,
                            player,
                            WordApi.audioAbsoluteUrl(url),
                            "WordDetailAudio",
                        )
                    }
                },
                onPlayVideo = onPlayContextVideo,
            )
            item { Spacer(Modifier.height(12.dp)) }
            item {
                NoteSection(
                    note = note,
                    loading = noteLoading,
                    error = noteError,
                    onEdit = { editingNote = true },
                )
            }
            item { Spacer(Modifier.height(12.dp)) }
            item {
                SectionTitle("英文释义")
            }
            renderEnglish(english)
            item { Spacer(Modifier.height(32.dp)) }
        }
    }
}

@Composable
private fun WordHeader(
    word: String,
    english: LoadState<LookupData>,
    level: Int,
    onPlay: (String) -> Unit,
) {
    Column {
        Row(verticalAlignment = Alignment.Top) {
            Text(
                text = word,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                color = MaterialTheme.colorScheme.onSurface,
            )
            if (level > 0) {
                Spacer(Modifier.width(4.dp))
                WordLevelBadge(level)
            }
        }
        Spacer(Modifier.height(6.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            when (english) {
                LoadState.Loading -> {
                    InlineLoading()
                }
                is LoadState.Failure -> {
                    Text(
                        text = "—",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontSize = 14.sp,
                    )
                }
                is LoadState.Success -> {
                    val usPron = english.data.pronunciations
                        .firstOrNull { it.audioUrl.isNotBlank() && it.accent.equals("US", ignoreCase = true) }
                        ?: english.data.pronunciations.firstOrNull { it.audioUrl.isNotBlank() }
                    if (usPron == null) {
                        Text(
                            text = "—",
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontSize = 14.sp,
                        )
                    } else {
                        Text(
                            text = "美",
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontSize = 12.sp,
                        )
                        if (usPron.text.isNotBlank()) {
                            Text(
                                text = usPron.text,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                fontSize = 13.sp,
                                modifier = Modifier.padding(start = 4.dp),
                            )
                        }
                        IconButton(onClick = { onPlay(WordApi.audioAbsoluteUrl(usPron.audioUrl)) }) {
                            Icon(
                                painter = painterResource(R.drawable.ic_volume),
                                contentDescription = "播放发音",
                                tint = MaterialTheme.colorScheme.primary,
                            )
                        }
                    }
                }
            }
        }
        if (english is LoadState.Success) {
            val meta = english.data
            if (meta.pos.isNotBlank() || meta.oxford == 1 || meta.tags.isNotEmpty()) {
                Spacer(Modifier.height(6.dp))
                WordMetaChips(
                    pos = meta.pos,
                    oxford = meta.oxford,
                    tags = meta.tags,
                )
            }
        }
    }
}

@Composable
private fun SectionTitle(title: String) {
    Text(
        text = title,
        fontWeight = FontWeight.SemiBold,
        fontSize = 15.sp,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = Modifier.padding(vertical = 4.dp),
    )
}

private fun LazyListScope.renderEnglish(state: LoadState<LookupData>) {
    when (state) {
        LoadState.Loading -> item { InlineLoading() }
        is LoadState.Failure -> item { InlineFailure(state.message) }
        is LoadState.Success -> {
            if (state.data.definitions.isEmpty()) {
                item { InlineEmpty() }
            } else {
                items(state.data.definitions) { def ->
                    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                        Text(
                            text = def.pos.ifBlank { "-" },
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontSize = 13.sp,
                            modifier = Modifier.widthIn(min = 56.dp),
                        )
                        Spacer(Modifier.width(8.dp))
                        Text(
                            text = def.text,
                            color = MaterialTheme.colorScheme.onSurface,
                            fontSize = 14.sp,
                            modifier = Modifier.weight(1f),
                        )
                    }
                }
            }
        }
    }
}

private fun LazyListScope.renderChinese(state: LoadState<MeaningData>) {
    when (state) {
        LoadState.Loading -> item { InlineLoading() }
        is LoadState.Failure -> item { InlineFailure(state.message) }
        is LoadState.Success -> {
            if (state.data.meanings.isEmpty()) {
                item { InlineEmpty() }
            } else {
                items(state.data.meanings) { m ->
                    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                        Text(
                            text = m.pos.ifBlank { "-" },
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontSize = 13.sp,
                            modifier = Modifier.widthIn(min = 56.dp),
                        )
                        Spacer(Modifier.width(8.dp))
                        Text(
                            text = m.meaning,
                            color = MaterialTheme.colorScheme.onSurface,
                            fontSize = 14.sp,
                            modifier = Modifier.weight(1f),
                        )
                    }
                }
            }
        }
    }
}

internal fun LazyListScope.renderContexts(
    state: LoadState<ContextsData>,
    onPlay: (String) -> Unit,
    onPlayVideo: (ContextVideo) -> Unit,
) {
    when (state) {
        LoadState.Loading -> item { InlineLoading() }
        is LoadState.Failure -> item { InlineFailure(state.message) }
        is LoadState.Success -> {
            if (state.data.contexts.isEmpty()) {
                item { InlineEmpty() }
            } else {
                items(state.data.contexts) { cx ->
                    Surface(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(vertical = 4.dp),
                        color = MaterialTheme.colorScheme.surfaceVariant,
                        shape = RoundedCornerShape(8.dp),
                    ) {
                        Column(modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        text = buildHighlightedSentence(
                                            sentence = cx.sentence,
                                            highlightStart = cx.highlightStart,
                                            highlightEnd = cx.highlightEnd,
                                            highlightStyle = SpanStyle(
                                                color = MaterialTheme.colorScheme.primary,
                                                fontWeight = FontWeight.Bold,
                                            ),
                                        ),
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        fontSize = 14.sp,
                                        lineHeight = 20.sp,
                                    )
                                    if (cx.translation.isNotBlank()) {
                                        Spacer(Modifier.height(4.dp))
                                        Text(
                                            text = cx.translation,
                                            color = MaterialTheme.colorScheme.onSurface,
                                            fontSize = 13.sp,
                                            lineHeight = 19.sp,
                                        )
                                    }
                                }
                                if (cx.audioUrl.isNotBlank() || cx.videos.isNotEmpty()) {
                                    Column(
                                        horizontalAlignment = Alignment.CenterHorizontally,
                                    ) {
                                        if (cx.audioUrl.isNotBlank()) {
                                            IconButton(
                                                onClick = { onPlay(cx.audioUrl) },
                                                modifier = Modifier.size(40.dp),
                                            ) {
                                                Icon(
                                                    painter = painterResource(R.drawable.ic_volume),
                                                    contentDescription = "播放语境",
                                                    tint = MaterialTheme.colorScheme.primary,
                                                )
                                            }
                                        }
                                        cx.videos.forEachIndexed { index, video ->
                                            IconButton(
                                                onClick = { onPlayVideo(video) },
                                                modifier = Modifier.size(40.dp),
                                            ) {
                                                Icon(
                                                    painter = painterResource(R.drawable.ic_video_play),
                                                    contentDescription = if (cx.videos.size == 1) {
                                                        "播放视频语境"
                                                    } else {
                                                        "播放视频语境 ${index + 1}"
                                                    },
                                                    tint = MaterialTheme.colorScheme.primary,
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
        }
    }
}

internal fun buildHighlightedSentence(
    sentence: String,
    highlightStart: Int,
    highlightEnd: Int,
    highlightStyle: SpanStyle,
): AnnotatedString {
    if (
        highlightStart < 0 ||
        highlightEnd <= highlightStart ||
        highlightEnd > sentence.length
    ) {
        return AnnotatedString(sentence)
    }
    return buildAnnotatedString {
        append(sentence.substring(0, highlightStart))
        withStyle(highlightStyle) {
            append(sentence.substring(highlightStart, highlightEnd))
        }
        append(sentence.substring(highlightEnd))
    }
}

@Composable
private fun InlineLoading() {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
    ) {
        Text(
            text = "加载中",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            fontSize = 13.sp,
        )
        Spacer(Modifier.width(8.dp))
        LoadingDots(color = MaterialTheme.colorScheme.onSurface)
    }
}

@Composable
private fun InlineFailure(message: String) {
    Text(
        text = message,
        color = MaterialTheme.colorScheme.error,
        fontSize = 13.sp,
        modifier = Modifier.padding(vertical = 6.dp),
    )
}

@Composable
private fun InlineEmpty() {
    Text(
        text = "暂无",
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        fontSize = 13.sp,
        modifier = Modifier.padding(vertical = 6.dp),
    )
}
