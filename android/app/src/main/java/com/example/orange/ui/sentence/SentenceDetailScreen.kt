package com.example.orange.ui.sentence

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.word.SentenceFavorite
import com.example.orange.data.word.SentenceTag
import com.example.orange.data.word.WordApi
import kotlinx.coroutines.launch

private sealed class SentenceDetailState {
    object Loading : SentenceDetailState()
    data class Success(val item: SentenceFavorite) : SentenceDetailState()
    object Failure : SentenceDetailState()
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun SentenceDetailScreen(
    sentenceId: Long,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var state by remember(sentenceId) {
        mutableStateOf<SentenceDetailState>(SentenceDetailState.Loading)
    }
    var loadTick by remember(sentenceId) { mutableIntStateOf(0) }
    var showTagManager by remember(sentenceId) { mutableStateOf(false) }
    var allTags by remember(sentenceId) { mutableStateOf<List<SentenceTag>?>(null) }
    var tagsLoading by remember(sentenceId) { mutableStateOf(false) }
    var tagLoadTick by remember(sentenceId) { mutableIntStateOf(0) }
    var tagsSaving by remember(sentenceId) { mutableStateOf(false) }
    var tagError by remember(sentenceId) { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(sentenceId, loadTick) {
        state = SentenceDetailState.Loading
        state = WordApi.sentenceFavoriteDetail(sentenceId).fold(
            onSuccess = { SentenceDetailState.Success(it) },
            onFailure = { SentenceDetailState.Failure },
        )
    }

    LaunchedEffect(showTagManager, tagLoadTick) {
        if (!showTagManager) return@LaunchedEffect
        tagsLoading = true
        tagError = null
        allTags = WordApi.sentenceTags().fold(
            onSuccess = { it },
            onFailure = {
                tagError = it.message ?: "标签加载失败"
                null
            },
        )
        tagsLoading = false
    }

    val currentItem = (state as? SentenceDetailState.Success)?.item
    BackHandler(onBack = onBack)
    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("句子详情") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            painter = painterResource(R.drawable.ic_back),
                            contentDescription = "返回",
                        )
                    }
                },
                actions = {
                    IconButton(
                        onClick = {
                            allTags = null
                            tagError = null
                            showTagManager = true
                        },
                        enabled = currentItem != null,
                    ) {
                        Icon(
                            painter = painterResource(R.drawable.ic_flag),
                            contentDescription = "设置标签与备注",
                        )
                    }
                },
            )
        },
    ) { innerPadding ->
        when (val current = state) {
            SentenceDetailState.Loading -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    CircularProgressIndicator()
                }
            }
            SentenceDetailState.Failure -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text("加载失败", color = MaterialTheme.colorScheme.error)
                    TextButton(onClick = { loadTick++ }) {
                        Text("重试")
                    }
                }
            }
            is SentenceDetailState.Success -> {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(innerPadding)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 20.dp, vertical = 18.dp),
                    verticalArrangement = Arrangement.spacedBy(24.dp),
                ) {
                    SentenceDetailSection(
                        title = "英文",
                        content = current.item.sentence,
                        contentSize = 20,
                    )
                    SentenceDetailSection(
                        title = "中文",
                        content = current.item.translation,
                        contentSize = 17,
                    )
                    SentenceDetailSection(
                        title = "备注",
                        content = current.item.note.ifBlank { "暂无备注" },
                        contentSize = 16,
                    )
                    SentenceTagsSection(current.item)
                }
            }
        }
    }

    if (showTagManager && currentItem != null) {
        ManageSentenceTagsDialog(
            allTags = allTags,
            currentTags = currentItem.tags,
            currentNote = currentItem.note,
            loading = tagsLoading,
            saving = tagsSaving,
            error = tagError,
            onRetry = { tagLoadTick++ },
            onDismiss = {
                if (!tagsSaving) {
                    showTagManager = false
                    tagError = null
                }
            },
            onConfirm = { tagIds, note ->
                tagsSaving = true
                tagError = null
                scope.launch {
                    WordApi.replaceSentenceFavoriteTags(sentenceId, tagIds, note).fold(
                        onSuccess = {
                            state = SentenceDetailState.Success(it)
                            tagsSaving = false
                            showTagManager = false
                        },
                        onFailure = {
                            tagError = it.message ?: "标签保存失败"
                            tagsSaving = false
                        },
                    )
                }
            },
        )
    }
}

@Composable
private fun SentenceTagsSection(item: SentenceFavorite) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = "标签",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
        )
        if (item.tags.isEmpty()) {
            Text(
                text = "暂无标签",
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                item.tags.forEach { tag ->
                    SentenceTagChip(
                        name = tag.name,
                        color = tag.color,
                        modifier = Modifier.widthIn(max = 140.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun SentenceDetailSection(
    title: String,
    content: String,
    contentSize: Int,
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = title,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
        )
        Text(
            text = content,
            color = MaterialTheme.colorScheme.onSurface,
            fontSize = contentSize.sp,
            lineHeight = (contentSize + 10).sp,
        )
    }
}
