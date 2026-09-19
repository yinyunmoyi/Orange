package com.example.orange.ui.wordlist

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.word.FavoriteGroupSummary
import com.example.orange.data.word.FavoriteItem
import com.example.orange.data.word.FavoritePageData
import com.example.orange.data.word.WordApi
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.ui.components.WordLevelBadge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

data class FavoriteGroupState(
    val summary: FavoriteGroupSummary,
    val expanded: Boolean = false,
    val items: List<FavoriteItem>,
    val nextCursor: String? = null,
    val loading: Boolean = false,
    val loadedOnce: Boolean = false,
    val error: String? = null,
)

internal fun applyFavoritePage(
    state: FavoriteGroupState,
    page: FavoritePageData,
): FavoriteGroupState {
    val existingKeys = state.items.mapTo(mutableSetOf()) { "${it.itemType}:${it.itemId}" }
    val appended = page.items.filter { existingKeys.add("${it.itemType}:${it.itemId}") }
    return state.copy(
        items = state.items + appended,
        nextCursor = page.nextCursor,
        loading = false,
        loadedOnce = true,
        error = null,
    )
}

private val rfc3339Format = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.getDefault()).apply {
    timeZone = java.util.TimeZone.getTimeZone("UTC")
}

private val dayFormat = SimpleDateFormat("M月d日", Locale.getDefault())
private val monthFormat = SimpleDateFormat("yyyy年M月", Locale.getDefault())

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WordListScreen(
    onItemClick: (FavoriteItem) -> Unit,
    onWordSearch: (String) -> Unit,
    modifier: Modifier = Modifier,
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
) {
    var groups by remember { mutableStateOf<List<FavoriteGroupState>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var errorMsg by remember { mutableStateOf<String?>(null) }
    var loadTick by remember { mutableStateOf(0) }
    var queryInput by remember { mutableStateOf("") }
    var searchState by remember { mutableStateOf<SearchState>(SearchState.Idle) }
    var searchTick by remember { mutableStateOf(0) }
    val scope = rememberCoroutineScope()
    val standaloneMode by StandaloneRepository.modeEnabled.collectAsState()
    val standaloneSnapshot by StandaloneRepository.snapshot.collectAsState()
    val localItems = standaloneSnapshot.wordItems

    fun updateGroup(id: String, transform: (FavoriteGroupState) -> FavoriteGroupState) {
        groups = groups.map { if (it.summary.id == id) transform(it) else it }
    }

    fun loadGroup(id: String) {
        val group = groups.firstOrNull { it.summary.id == id } ?: return
        if (group.loading || (group.loadedOnce && group.nextCursor == null)) return
        val cursor = group.nextCursor
        updateGroup(id) { it.copy(loading = true, error = null) }
        scope.launch {
            WordApi.favoritesPage(
                startAt = group.summary.rangeStart,
                endAt = group.summary.rangeEnd,
                cursor = cursor,
            ).fold(
                onSuccess = { page ->
                    updateGroup(id) { current -> applyFavoritePage(current, page) }
                },
                onFailure = { error ->
                    updateGroup(id) {
                        it.copy(loading = false, error = error.message ?: error.javaClass.simpleName)
                    }
                },
            )
        }
    }

    LaunchedEffect(loadTick, standaloneMode, standaloneSnapshot.pendingCount) {
        if (standaloneMode) {
            groups = emptyList()
            loading = false
            errorMsg = null
            return@LaunchedEffect
        }
        loading = true
        errorMsg = null
        WordApi.favoriteGroups().onSuccess { data ->
            groups = data.groups.map { FavoriteGroupState(summary = it, items = emptyList()) }
            loading = false
        }.onFailure { t ->
            errorMsg = t.message ?: t.javaClass.simpleName
            loading = false
        }
    }

    LaunchedEffect(queryInput, searchTick, standaloneMode, localItems) {
        val trimmed = queryInput.trim()
        if (trimmed.isEmpty()) {
            searchState = SearchState.Idle
            return@LaunchedEffect
        }
        if (!searchQueryPattern.matches(trimmed)) {
            searchState = SearchState.Idle
            return@LaunchedEffect
        }
        val matchedLocal = localItems.filter { it.text.contains(trimmed, ignoreCase = true) }
        if (standaloneMode) {
            searchState = SearchState.Success(matchedLocal)
            return@LaunchedEffect
        }
        delay(250)
        searchState = SearchState.Loading
        WordApi.favorites(q = trimmed).fold(
            onSuccess = { data ->
                if (queryInput.trim() == trimmed) {
                    searchState = SearchState.Success(
                        com.example.orange.data.standalone.mergeStandaloneWords(data.items, matchedLocal),
                    )
                }
            },
            onFailure = { t ->
                if (queryInput.trim() == trimmed) {
                    searchState = if (matchedLocal.isNotEmpty()) {
                        SearchState.Success(matchedLocal)
                    } else {
                        SearchState.Error(t.message ?: t.javaClass.simpleName)
                    }
                }
            },
        )
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("单词本") },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(bottom = bottomPadding),
        ) {
            OutlinedTextField(
                value = queryInput,
                onValueChange = { queryInput = it },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                singleLine = true,
                placeholder = { Text("搜索单词或短语") },
                leadingIcon = {
                    Text(
                        text = "🔍",
                        modifier = Modifier.padding(start = 12.dp),
                    )
                },
                trailingIcon = {
                    if (queryInput.isNotEmpty()) {
                        TextButton(onClick = { queryInput = "" }) {
                            Text("清除")
                        }
                    }
                },
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(
                    onSearch = {
                        if (!standaloneMode) {
                            normalizeWordSearchTarget(queryInput)?.let(onWordSearch)
                        }
                    },
                ),
                shape = RoundedCornerShape(24.dp),
            )
            Box(modifier = Modifier.fillMaxSize()) {
                if (queryInput.trim().isNotEmpty()) {
                    SearchResultView(
                        state = searchState,
                        onItemClick = onItemClick,
                        onRetry = { searchTick++ },
                    )
                } else {
                    when {
                        standaloneMode -> {
                            PendingWordList(
                                items = localItems,
                                emptyText = "还没有未同步的单词或短语",
                                onItemClick = onItemClick,
                            )
                        }
                        loading -> {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                CircularProgressIndicator()
                            }
                        }
                        errorMsg != null && localItems.isEmpty() -> {
                            Column(
                                modifier = Modifier.fillMaxSize(),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.Center,
                            ) {
                                Text(
                                    text = "加载失败：${errorMsg}",
                                    color = MaterialTheme.colorScheme.error,
                                    style = MaterialTheme.typography.bodyMedium,
                                )
                                TextButton(onClick = { loadTick++ }) {
                                    Text("重试")
                                }
                            }
                        }
                        groups.isEmpty() && localItems.isEmpty() -> {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                Text(
                                    text = "还没有收藏单词或短语\n在阅读中选中内容即可收藏",
                                    style = MaterialTheme.typography.bodyLarge,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                        else -> {
                            LazyColumn(
                                modifier = Modifier.fillMaxSize(),
                                contentPadding = PaddingValues(vertical = 8.dp),
                            ) {
                                if (localItems.isNotEmpty()) {
                                    item(key = "pending-header") {
                                        Text(
                                            text = "未同步",
                                            style = MaterialTheme.typography.titleSmall,
                                            fontWeight = FontWeight.SemiBold,
                                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                                        )
                                    }
                                    items(localItems, key = { "pending-${it.clientId}" }) { item ->
                                        WordItemRow(item = item, onClick = { onItemClick(item) })
                                    }
                                    item(key = "pending-divider") { HorizontalDivider() }
                                }
                                groups.forEach { group ->
                                    item(key = "header-${group.summary.id}") {
                                        GroupHeader(
                                            title = favoriteGroupTitle(group.summary),
                                            count = group.summary.count,
                                            expanded = group.expanded,
                                            onClick = {
                                                val expanding = !group.expanded
                                                updateGroup(group.summary.id) { it.copy(expanded = expanding) }
                                                if (expanding && !group.loadedOnce && !group.loading) {
                                                    loadGroup(group.summary.id)
                                                }
                                            },
                                        )
                                    }
                                    if (group.expanded) {
                                        items(group.items, key = { "${group.summary.id}-${it.itemType}-${it.itemId}" }) { item ->
                                            WordItemRow(
                                                item = item,
                                                onClick = { onItemClick(item) },
                                            )
                                        }
                                        if (group.loading) {
                                            item(key = "loading-${group.summary.id}-${group.items.size}") {
                                                Box(
                                                    modifier = Modifier.fillMaxWidth().padding(16.dp),
                                                    contentAlignment = Alignment.Center,
                                                ) {
                                                    CircularProgressIndicator()
                                                }
                                            }
                                        } else if (group.error != null) {
                                            item(key = "error-${group.summary.id}") {
                                                Row(
                                                    modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 8.dp),
                                                    verticalAlignment = Alignment.CenterVertically,
                                                ) {
                                                    Text(
                                                        text = "加载失败：${group.error}",
                                                        color = MaterialTheme.colorScheme.error,
                                                        modifier = Modifier.weight(1f),
                                                    )
                                                    TextButton(onClick = { loadGroup(group.summary.id) }) {
                                                        Text("重试")
                                                    }
                                                }
                                            }
                                        } else if (group.loadedOnce && group.nextCursor != null) {
                                            item(key = "more-${group.summary.id}-${group.items.size}") {
                                                LaunchedEffect(group.summary.id, group.items.size, group.nextCursor) {
                                                    loadGroup(group.summary.id)
                                                }
                                                Box(
                                                    modifier = Modifier.fillMaxWidth().padding(12.dp),
                                                    contentAlignment = Alignment.Center,
                                                ) {
                                                    CircularProgressIndicator()
                                                }
                                            }
                                        }
                                        item(key = "divider-${group.summary.id}") {
                                            HorizontalDivider(
                                                modifier = Modifier.padding(horizontal = 16.dp),
                                                thickness = 0.5.dp,
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

@Composable
private fun PendingWordList(
    items: List<FavoriteItem>,
    emptyText: String,
    onItemClick: (FavoriteItem) -> Unit,
) {
    if (items.isEmpty()) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Text(text = emptyText, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        return
    }
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(vertical = 8.dp),
    ) {
        items(items, key = { it.clientId ?: "${it.itemType}:${it.itemId}" }) { item ->
            WordItemRow(item = item, onClick = { onItemClick(item) })
        }
    }
}

private val searchQueryPattern = Regex("^[a-zA-Z][a-zA-Z' -]{0,79}$")
private val singleWordSearchPattern = Regex("^[a-zA-Z][a-zA-Z'-]*$")

internal fun normalizeWordSearchTarget(query: String): String? {
    val normalized = query.trim()
    return normalized
        .takeIf(singleWordSearchPattern::matches)
        ?.lowercase(Locale.ROOT)
}

internal sealed class SearchState {
    object Idle : SearchState()
    object Loading : SearchState()
    data class Success(val items: List<FavoriteItem>) : SearchState()
    data class Error(val message: String) : SearchState()
}

@Composable
private fun SearchResultView(
    state: SearchState,
    onItemClick: (FavoriteItem) -> Unit,
    onRetry: () -> Unit,
) {
    when (state) {
        SearchState.Idle -> {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(
                    text = "请输入以字母开头的关键字",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        SearchState.Loading -> {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                CircularProgressIndicator()
            }
        }
        is SearchState.Error -> {
            Column(
                modifier = Modifier.fillMaxSize(),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center,
            ) {
                Text(
                    text = "搜索失败：${state.message}",
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                )
                TextButton(onClick = onRetry) {
                    Text("重试")
                }
            }
        }
        is SearchState.Success -> {
            if (state.items.isEmpty()) {
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Text(
                        text = "没有匹配的收藏词",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(vertical = 8.dp),
                ) {
                    items(
                        state.items,
                        key = { "search-${it.clientId ?: "${it.itemType}-${it.itemId}"}" },
                    ) { item ->
                        WordItemRow(
                            item = item,
                            onClick = { onItemClick(item) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun GroupHeader(
    title: String,
    count: Int,
    expanded: Boolean,
    onClick: () -> Unit,
) {
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        tonalElevation = 0.dp,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = if (expanded) "▼" else "▶",
                fontSize = 10.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(modifier = Modifier.width(8.dp))
            Text(
                text = title,
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f),
            )
            Text(
                text = "$count",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun WordItemRow(
    item: FavoriteItem,
    onClick: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = item.clientId == null && item.itemId > 0L, onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 10.dp)
            .padding(start = 28.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = item.text,
                style = MaterialTheme.typography.bodyLarge,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f, fill = false),
            )
            if (item.itemType == "phrase") {
                Spacer(modifier = Modifier.width(8.dp))
                Surface(
                    shape = RoundedCornerShape(4.dp),
                    color = MaterialTheme.colorScheme.primaryContainer,
                ) {
                    Text(
                        text = "短语",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                    )
                }
            }
            if (item.clientId != null) {
                Spacer(modifier = Modifier.width(8.dp))
                Surface(
                    shape = RoundedCornerShape(4.dp),
                    color = MaterialTheme.colorScheme.tertiaryContainer,
                ) {
                    Text(
                        text = "未同步",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onTertiaryContainer,
                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                    )
                }
            }
            if (item.topMeaningPos.isNotEmpty()) {
                Spacer(modifier = Modifier.width(8.dp))
                Surface(
                    shape = RoundedCornerShape(4.dp),
                    color = MaterialTheme.colorScheme.secondaryContainer,
                ) {
                    Text(
                        text = item.topMeaningPos,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSecondaryContainer,
                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                    )
                }
            }
            if (item.itemType == "word" && item.level > 0) {
                Spacer(modifier = Modifier.width(8.dp))
                WordLevelBadge(level = item.level)
            }
        }
        if (item.topMeaningText.isNotEmpty()) {
            Text(
                text = item.topMeaningText,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 2.dp),
            )
        }
    }
}

internal fun favoriteGroupTitle(group: FavoriteGroupSummary): String {
    val start = parseFavoriteDate(group.displayStart) ?: return group.id
    val end = parseFavoriteDate(group.displayEnd) ?: start
    return when (group.type) {
        "day" -> dayFormat.format(start)
        "week" -> "${dayFormat.format(start)} - ${dayFormat.format(end)}"
        "month" -> monthFormat.format(start)
        else -> group.id
    }
}

private fun parseFavoriteDate(s: String): Date? {
    runCatching {
        SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssXXX", Locale.getDefault()).parse(s.trim())
    }.getOrNull()?.let { return it }
    return runCatching {
        val normalized = s.trim()
            .replace("Z$".toRegex(), "+0000")
            .let { if (it.contains('+') || it.contains("GMT")) it else "${it}+0000" }
        val alt = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssZ", Locale.getDefault())
        alt.parse(normalized)
    }.getOrNull() ?: runCatching { rfc3339Format.parse(s) }.getOrNull()
}
