package com.example.orange.ui.wordlist

import android.media.AudioAttributes
import android.media.MediaPlayer
import com.example.orange.data.logging.AppLog as Log
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.audiosync.CachedAudioPlayer
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.ContextsData
import com.example.orange.data.word.PhraseDetailData
import com.example.orange.data.word.WordApi
import com.example.orange.ui.note.NoteEditScreen
import com.example.orange.ui.note.NoteSection
import kotlinx.coroutines.launch

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun PhraseDetailScreen(
    itemId: Long,
    fallbackText: String,
    onBack: () -> Unit,
    onPlayContextVideo: (ContextVideo) -> Unit = {},
    modifier: Modifier = Modifier,
) {
    var detail by remember(itemId) { mutableStateOf<LoadState<PhraseDetailData>>(LoadState.Loading) }
    var note by remember(itemId) { mutableStateOf<String?>(null) }
    var noteLoading by remember(itemId) { mutableStateOf(true) }
    var noteError by remember(itemId) { mutableStateOf<String?>(null) }
    var editingNote by remember(itemId) { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    LaunchedEffect(itemId, fallbackText) {
        kotlinx.coroutines.supervisorScope {
            launch {
                detail = WordApi.phraseDetail(itemId).fold(
                    onSuccess = { LoadState.Success(it) },
                    onFailure = {
                        Log.e("PhraseDetail", "load failed itemId=$itemId", it)
                        LoadState.Failure("加载失败")
                    },
                )
            }
            launch {
                noteLoading = true
                noteError = null
                WordApi.getNote("phrase", fallbackText).fold(
                    onSuccess = { note = it.note },
                    onFailure = {
                        Log.w("PhraseDetail", "note failed text=$fallbackText err=${it.message}", it)
                        noteError = "备注加载失败"
                    },
                )
                noteLoading = false
            }
        }
    }

    if (editingNote) {
        NoteEditScreen(
            itemType = "phrase",
            text = (detail as? LoadState.Success)?.data?.phrase?.ifBlank { fallbackText } ?: fallbackText,
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
                    .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
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
                title = { Text("短语详情") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(painterResource(R.drawable.ic_back), contentDescription = "返回")
                    }
                },
                actions = {
                    FavoriteDeleteAction(
                        itemKey = "phrase:$itemId",
                        deleteFavorite = { WordApi.deleteFavorite("phrase", itemId) },
                        onDeleted = onBack,
                        onDeleteFailed = { error ->
                            Log.e("PhraseDetail", "delete failed itemId=$itemId", error)
                            scope.launch {
                                snackbarHostState.showSnackbar("删除失败，请重试")
                            }
                        },
                    )
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        when (val state = detail) {
            LoadState.Loading -> {
                Column(
                    Modifier.fillMaxSize().padding(padding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    CircularProgressIndicator()
                }
            }
            is LoadState.Failure -> {
                Column(
                    Modifier.fillMaxSize().padding(padding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text(state.message, color = MaterialTheme.colorScheme.error)
                }
            }
            is LoadState.Success -> {
                val data = state.data
                LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(padding),
                    contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    item {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                text = data.phrase,
                                style = MaterialTheme.typography.headlineSmall,
                                fontWeight = FontWeight.Bold,
                                modifier = Modifier.weight(1f),
                            )
                            if (data.audioUrl.isNotBlank()) {
                                IconButton(
                                    onClick = {
                                        scope.launch {
                                            CachedAudioPlayer.play(
                                                context,
                                                player,
                                                WordApi.audioAbsoluteUrl(data.audioUrl),
                                                "PhraseDetailAudio",
                                            )
                                        }
                                    },
                                ) {
                                    Icon(
                                        painterResource(R.drawable.ic_volume),
                                        contentDescription = "播放短语",
                                        tint = MaterialTheme.colorScheme.primary,
                                    )
                                }
                            }
                        }
                    }
                    item { Spacer(Modifier.height(12.dp)) }
                    item { PhraseSectionTitle("中文释义") }
                    if (data.meanings.isEmpty()) {
                        item {
                            Text("暂无内容", color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    } else {
                        items(data.meanings) { meaning ->
                            Row(Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                                Text(
                                    text = meaning.pos.ifBlank { "-" },
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    fontSize = 13.sp,
                                    modifier = Modifier.widthIn(min = 56.dp),
                                )
                                Spacer(Modifier.width(8.dp))
                                Text(
                                    text = meaning.meaning,
                                    fontSize = 14.sp,
                                    modifier = Modifier.weight(1f),
                                )
                            }
                        }
                    }
                    item { Spacer(Modifier.height(12.dp)) }
                    item { PhraseSectionTitle("语境") }
                    renderContexts(
                        state = LoadState.Success(ContextsData(data.phrase, data.contexts)),
                        onPlay = { url ->
                            scope.launch {
                                CachedAudioPlayer.play(
                                    context,
                                    player,
                                    WordApi.audioAbsoluteUrl(url),
                                    "PhraseDetailAudio",
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
                    item { Spacer(Modifier.height(32.dp)) }
                }
            }
        }
    }
}

@Composable
private fun PhraseSectionTitle(text: String) {
    Text(
        text = text,
        fontWeight = FontWeight.SemiBold,
        fontSize = 15.sp,
        modifier = Modifier.padding(vertical = 4.dp),
    )
}
