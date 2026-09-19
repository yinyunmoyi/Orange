package com.example.orange.ui.sentence

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.data.standalone.mergeStandaloneSentences
import com.example.orange.data.word.SentenceFavorite
import com.example.orange.data.word.SentenceTag
import com.example.orange.data.word.WordApi
import kotlinx.coroutines.launch

internal fun sentenceFavoriteIdForOpen(item: SentenceFavorite): Long? =
    item.id.takeIf { it > 0L && item.clientId == null && item.sentence.isNotBlank() }

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun SentenceListScreen(
    onSentenceClick: (Long) -> Unit,
    modifier: Modifier = Modifier,
    bottomPadding: Dp = 0.dp,
) {
    var sentenceItems by remember { mutableStateOf<List<SentenceFavorite>>(emptyList()) }
    var tags by remember { mutableStateOf<List<SentenceTag>>(emptyList()) }
    var selectedTagIds by remember { mutableStateOf<Set<Long>>(emptySet()) }
    var sentencesLoading by remember { mutableStateOf(true) }
    var sentencesFiltering by remember { mutableStateOf(false) }
    var sentencesLoaded by remember { mutableStateOf(false) }
    var sentenceError by remember { mutableStateOf<String?>(null) }
    var tagsLoading by remember { mutableStateOf(true) }
    var tagError by remember { mutableStateOf<String?>(null) }
    var sentenceLoadTick by remember { mutableIntStateOf(0) }
    var tagLoadTick by remember { mutableIntStateOf(0) }
    var showCreateTag by remember { mutableStateOf(false) }
    var creatingTag by remember { mutableStateOf(false) }
    var createTagError by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    val standaloneMode by StandaloneRepository.modeEnabled.collectAsState()
    val standaloneSnapshot by StandaloneRepository.snapshot.collectAsState()
    val localSentences = standaloneSnapshot.sentenceItems

    LaunchedEffect(tagLoadTick, standaloneMode) {
        if (standaloneMode) {
            tags = emptyList()
            selectedTagIds = emptySet()
            tagsLoading = false
            tagError = null
            return@LaunchedEffect
        }
        tagsLoading = true
        tagError = null
        WordApi.sentenceTags().fold(
            onSuccess = {
                tags = it
                val validIDs = it.mapTo(hashSetOf()) { tag -> tag.id }
                selectedTagIds = selectedTagIds.intersect(validIDs)
            },
            onFailure = {
                tagError = it.message ?: it.javaClass.simpleName
            },
        )
        tagsLoading = false
    }

    LaunchedEffect(selectedTagIds, sentenceLoadTick, standaloneMode, localSentences) {
        if (standaloneMode) {
            sentenceItems = localSentences
            sentencesLoading = false
            sentencesFiltering = false
            sentencesLoaded = true
            sentenceError = null
            return@LaunchedEffect
        }
        if (sentencesLoaded) {
            sentencesFiltering = true
        } else {
            sentencesLoading = true
        }
        sentenceError = null
        WordApi.sentenceFavorites(selectedTagIds).fold(
            onSuccess = {
                sentenceItems = if (selectedTagIds.isEmpty()) {
                    mergeStandaloneSentences(it.items, localSentences)
                } else {
                    it.items
                }
                sentencesLoaded = true
            },
            onFailure = {
                if (selectedTagIds.isEmpty() && localSentences.isNotEmpty()) {
                    sentenceItems = localSentences
                    sentenceError = null
                } else {
                    sentenceError = it.message ?: it.javaClass.simpleName
                }
            },
        )
        sentencesLoading = false
        sentencesFiltering = false
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("句子") },
                actions = {
                    if (!standaloneMode) {
                        IconButton(onClick = {
                            createTagError = null
                            showCreateTag = true
                        }) {
                            Icon(
                                painter = painterResource(R.drawable.ic_flag),
                                contentDescription = "新增标签",
                            )
                        }
                    }
                },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(bottom = bottomPadding),
        ) {
            when {
                tagsLoading -> {
                    LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                }
                tagError != null -> {
                    TextButton(
                        onClick = { tagLoadTick++ },
                        modifier = Modifier.padding(horizontal = 8.dp),
                    ) {
                        Text("标签加载失败，点击重试")
                    }
                }
                tags.isNotEmpty() -> {
                    SentenceTagFilterRow(
                        tags = tags,
                        selectedTagIds = selectedTagIds,
                        onTagClick = {
                            selectedTagIds = toggleSentenceTagSelection(selectedTagIds, it)
                        },
                    )
                }
            }
            if (sentencesFiltering) {
                LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
            }
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f),
            ) {
                when {
                    sentencesLoading -> CircularProgressIndicator(Modifier.align(Alignment.Center))
                    sentenceError != null -> {
                        Column(
                            modifier = Modifier.align(Alignment.Center),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Text(
                                text = "加载失败",
                                color = MaterialTheme.colorScheme.error,
                            )
                            TextButton(onClick = { sentenceLoadTick++ }) {
                                Text("重试")
                            }
                        }
                    }
                    sentenceItems.isEmpty() -> {
                        Text(
                            text = if (selectedTagIds.isEmpty()) {
                                "还没有收藏句子\n在阅读时选中句子并分析即可收藏"
                            } else {
                                "没有匹配这些标签的句子"
                            },
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        LazyColumn(
                            modifier = Modifier.fillMaxSize(),
                            contentPadding = PaddingValues(vertical = 4.dp),
                        ) {
                            items(
                                items = sentenceItems,
                                key = { it.clientId ?: "server-${it.id}" },
                            ) { item ->
                                SentenceListItem(
                                    item = item,
                                    selectedTagIds = selectedTagIds,
                                    onTagClick = {
                                        selectedTagIds = toggleSentenceTagSelection(
                                            selectedTagIds,
                                            it,
                                        )
                                    },
                                    onClick = {
                                        sentenceFavoriteIdForOpen(item)?.let(onSentenceClick)
                                    },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (showCreateTag) {
        CreateSentenceTagDialog(
            saving = creatingTag,
            error = createTagError,
            onDismiss = { if (!creatingTag) showCreateTag = false },
            onConfirm = { name, color ->
                creatingTag = true
                createTagError = null
                scope.launch {
                    WordApi.createSentenceTag(name, color).fold(
                        onSuccess = {
                            tags = tags + it
                            creatingTag = false
                            showCreateTag = false
                        },
                        onFailure = {
                            createTagError = it.message ?: "新增标签失败"
                            creatingTag = false
                        },
                    )
                }
            },
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SentenceTagFilterRow(
    tags: List<SentenceTag>,
    selectedTagIds: Set<Long>,
    onTagClick: (Long) -> Unit,
) {
    FlowRow(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalArrangement = Arrangement.spacedBy(8.dp),
        maxItemsInEachRow = 4,
    ) {
        tags.forEach { tag ->
            SentenceTagChip(
                name = tag.name,
                color = tag.color,
                selected = tag.id in selectedTagIds,
                showCheckbox = true,
                onClick = { onTagClick(tag.id) },
            )
        }
    }
}

@Composable
private fun SentenceListItem(
    item: SentenceFavorite,
    selectedTagIds: Set<Long>,
    onTagClick: (Long) -> Unit,
    onClick: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(vertical = 14.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(
            text = item.sentence,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            fontSize = 16.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(horizontal = 16.dp),
        )
        if (item.clientId != null) {
            Text(
                text = "未同步",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.tertiary,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
        }
        if (item.tags.isNotEmpty()) {
            LazyRow(
                contentPadding = PaddingValues(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(item.tags, key = { it.id }) { tag ->
                    SentenceTagBadge(
                        name = tag.name,
                        color = tag.color,
                        selected = tag.id in selectedTagIds,
                        onClick = { onTagClick(tag.id) },
                    )
                }
            }
        }
    }
    HorizontalDivider()
}
