package com.example.orange.ui.bookshelf

import android.graphics.BitmapFactory
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.Book
import com.example.orange.data.BookRepository
import com.example.orange.data.EpubImporter
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.data.standalone.StandaloneSyncScheduler
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BookshelfScreen(
    books: List<Book>,
    onBookClick: (Book) -> Unit,
    modifier: Modifier = Modifier,
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = remember { SnackbarHostState() }
    var importing by remember { mutableStateOf(false) }
    var editMode by rememberSaveable { mutableStateOf(false) }
    val selectedIds = remember { mutableStateListOf<String>() }
    val standaloneMode by StandaloneRepository.modeEnabled.collectAsState()
    val standaloneSnapshot by StandaloneRepository.snapshot.collectAsState()
    val standaloneSyncing by StandaloneRepository.syncInProgress.collectAsState()

    LaunchedEffect(editMode) {
        if (!editMode) selectedIds.clear()
    }
    LaunchedEffect(books) {
        if (editMode) {
            val validIds = books.map { it.id }.toSet()
            selectedIds.retainAll { it in validIds }
        }
    }

    val launcher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.OpenDocument(),
    ) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        if (importing) return@rememberLauncherForActivityResult
        importing = true
        scope.launch {
            try {
                val book = withContext(Dispatchers.IO) {
                    EpubImporter.importEpub(context, uri)
                }
                BookRepository.addImportedBook(book)
                snackbarHostState.showSnackbar("导入成功：${book.title}")
            } catch (t: Throwable) {
                snackbarHostState.showSnackbar("导入失败：${t.message ?: t.javaClass.simpleName}")
            } finally {
                importing = false
            }
        }
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("书架")
                        if (standaloneSyncing || standaloneSnapshot.pendingCount > 0) {
                            Text(
                                text = if (standaloneSyncing) {
                                    "正在同步"
                                } else {
                                    "${standaloneSnapshot.pendingCount} 项未同步"
                                },
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                },
                actions = {
                    Text(
                        text = "单体模式",
                        style = MaterialTheme.typography.labelMedium,
                    )
                    Switch(
                        checked = standaloneMode,
                        onCheckedChange = { enabled ->
                            StandaloneRepository.setEnabled(enabled)
                            if (!enabled) StandaloneSyncScheduler.enqueue(context)
                        },
                        modifier = Modifier.padding(horizontal = 8.dp),
                    )
                    TextButton(
                        onClick = { editMode = !editMode },
                        enabled = !importing,
                    ) {
                        Text(if (editMode) "完成" else "编辑")
                    }
                    if (importing) {
                        CircularProgressIndicator(
                            modifier = Modifier
                                .padding(end = 16.dp)
                                .size(20.dp),
                            strokeWidth = 2.dp,
                        )
                    } else {
                        TextButton(
                            onClick = {
                                launcher.launch(
                                    arrayOf(
                                        "application/epub+zip",
                                        "application/octet-stream",
                                        "*/*",
                                    )
                                )
                            },
                        ) {
                            Text("导入")
                        }
                    }
                },
            )
        },
        bottomBar = {
            if (editMode) {
                EditBottomBar(
                    selectedCount = selectedIds.size,
                    bottomPadding = bottomPadding,
                    onDelete = {
                        val idsSnapshot = selectedIds.toList()
                        if (idsSnapshot.isEmpty()) return@EditBottomBar
                        scope.launch {
                            withContext(Dispatchers.IO) {
                                idsSnapshot.forEach { BookRepository.removeBook(it) }
                            }
                            selectedIds.clear()
                            editMode = false
                            snackbarHostState.showSnackbar("已删除 ${idsSnapshot.size} 本")
                        }
                    },
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { innerPadding ->
        LazyVerticalGrid(
            columns = GridCells.Fixed(3),
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(horizontal = 12.dp),
            contentPadding = PaddingValues(
                start = 0.dp,
                end = 0.dp,
                top = 12.dp,
                bottom = 12.dp + bottomPadding,
            ),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            items(books, key = { it.id }) { book ->
                BookItem(
                    book = book,
                    editMode = editMode,
                    selected = book.id in selectedIds,
                    onClick = { onBookClick(book) },
                    onToggleSelect = {
                        if (book.id in selectedIds) selectedIds.remove(book.id) else selectedIds.add(book.id)
                    },
                )
            }
        }
    }
}

@Composable
private fun EditBottomBar(
    selectedCount: Int,
    onDelete: () -> Unit,
    bottomPadding: androidx.compose.ui.unit.Dp = 0.dp,
) {
    Surface(tonalElevation = 4.dp) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 6.dp)
                .padding(bottom = bottomPadding),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                text = "已选 $selectedCount 项",
                style = MaterialTheme.typography.bodyMedium,
            )
            TextButton(
                onClick = onDelete,
                enabled = selectedCount > 0,
            ) {
                Text(
                    text = "删除",
                    color = if (selectedCount > 0) MaterialTheme.colorScheme.error else Color.Unspecified,
                )
            }
        }
    }
}

@Composable
private fun BookItem(
    book: Book,
    editMode: Boolean,
    selected: Boolean,
    onClick: () -> Unit,
    onToggleSelect: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = if (editMode) onToggleSelect else onClick),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        BookCover(
            book = book,
            editMode = editMode,
            selected = selected,
            onToggleSelect = onToggleSelect,
        )
        Text(
            text = book.title,
            style = MaterialTheme.typography.bodyMedium,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(top = 8.dp),
        )
        Text(
            text = book.author,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun BookCover(
    book: Book,
    editMode: Boolean,
    selected: Boolean,
    onToggleSelect: () -> Unit,
) {
    val coverBitmap = remember(book.coverPath) {
        book.coverPath
            ?.takeIf { File(it).exists() }
            ?.let { runCatching { BitmapFactory.decodeFile(it) }.getOrNull() }
    }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .aspectRatio(3f / 4f)
            .clip(RoundedCornerShape(6.dp))
            .background(book.coverColor),
        contentAlignment = Alignment.Center,
    ) {
        if (coverBitmap != null) {
            Image(
                bitmap = coverBitmap.asImageBitmap(),
                contentDescription = book.title,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            Text(
                text = book.title,
                color = Color.White,
                fontWeight = FontWeight.Bold,
                fontSize = 16.sp,
                textAlign = TextAlign.Center,
                modifier = Modifier.padding(8.dp),
            )
        }
        if (editMode) {
            SelectionCheckbox(
                selected = selected,
                onClick = onToggleSelect,
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .padding(6.dp),
            )
        }
    }
}

@Composable
private fun SelectionCheckbox(
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Box(
        modifier = modifier
            .size(22.dp)
            .clip(RoundedCornerShape(4.dp))
            .background(Color.White.copy(alpha = 0.85f))
            .border(1.dp, Color.White, RoundedCornerShape(4.dp))
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        if (selected) {
            Text(
                text = "\u2713",
                color = MaterialTheme.colorScheme.primary,
                fontSize = 16.sp,
                fontWeight = FontWeight.Bold,
            )
        }
    }
}
