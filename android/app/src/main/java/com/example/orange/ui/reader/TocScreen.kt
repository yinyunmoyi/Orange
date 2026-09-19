package com.example.orange.ui.reader

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.Book
import com.example.orange.data.BookRepository
import com.example.orange.data.BookSource
import com.example.orange.data.EpubImporter
import com.example.orange.data.TocEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

private val TITLE_TAG_REGEX = Regex("<title[^>]*>([\\s\\S]*?)</title>", RegexOption.IGNORE_CASE)
private val H1_TAG_REGEX = Regex("<h1[^>]*>([\\s\\S]*?)</h1>", RegexOption.IGNORE_CASE)
private val H2_TAG_REGEX = Regex("<h2[^>]*>([\\s\\S]*?)</h2>", RegexOption.IGNORE_CASE)
private val TAG_STRIP_REGEX = Regex("<[^>]+>")
private val WHITESPACE_REGEX = Regex("\\s+")

@Composable
fun TocScreen(
    book: Book,
    currentIndex: Int,
    palette: ReaderPalette,
    onBack: () -> Unit,
    onChapterSelected: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val entries by produceState(
        initialValue = book.tocEntries,
        key1 = book.id,
    ) {
        if (value.isNotEmpty()) return@produceState
        val rebuilt = withContext(Dispatchers.IO) { EpubImporter.rebuildTocEntries(book) }
        if (rebuilt.isNotEmpty()) {
            BookRepository.updateTocEntries(book.id, rebuilt)
            value = rebuilt
            return@produceState
        }
        val fallback = withContext(Dispatchers.IO) { fallbackTitles(book) }
        value = fallback
    }

    val listState = rememberLazyListState()
    LaunchedEffect(book.id, currentIndex, entries) {
        if (entries.isEmpty()) return@LaunchedEffect
        val targetListIndex = entries.indexOfLast { it.spineIndex <= currentIndex }
            .coerceAtLeast(0)
        listState.scrollToItem(targetListIndex)
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .background(palette.bg)
            .statusBarsPadding()
            .navigationBarsPadding(),
    ) {
        TocTopBar(palette = palette, onBack = onBack)
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .background(palette.bg),
            state = listState,
            contentPadding = PaddingValues(vertical = 4.dp),
        ) {
            itemsIndexed(entries) { _, entry ->
                TocRow(
                    entry = entry,
                    isCurrent = entry.spineIndex == currentIndex,
                    palette = palette,
                    onClick = { onChapterSelected(entry.spineIndex) },
                )
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(1.dp)
                        .background(palette.fg.copy(alpha = 0.12f)),
                )
            }
        }
    }
}

@Composable
private fun TocTopBar(palette: ReaderPalette, onBack: () -> Unit) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(palette.bg)
            .padding(horizontal = 12.dp, vertical = 12.dp),
    ) {
        Text(
            text = "返回",
            color = palette.fg,
            fontSize = 16.sp,
            modifier = Modifier
                .align(Alignment.CenterStart)
                .clickable(onClick = onBack)
                .padding(horizontal = 8.dp, vertical = 4.dp),
        )
        Text(
            text = "目录",
            color = palette.fg,
            fontSize = 18.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.align(Alignment.Center),
        )
    }
}

@Composable
private fun TocRow(
    entry: TocEntry,
    isCurrent: Boolean,
    palette: ReaderPalette,
    onClick: () -> Unit,
) {
    val bg = if (isCurrent) palette.accent.copy(alpha = 0.16f) else Color.Transparent
    val textColor = if (isCurrent) palette.accent else palette.fg
    val leftPad = 20.dp + (entry.depth * 16).dp
    val weight = if (entry.depth == 0) FontWeight.SemiBold else FontWeight.Normal
    val size = if (entry.depth == 0) 16.sp else 15.sp

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .clickable(onClick = onClick)
            .padding(start = leftPad, end = 20.dp, top = 14.dp, bottom = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = entry.title,
            color = textColor,
            fontSize = size,
            fontWeight = weight,
        )
    }
}

private fun fallbackTitles(book: Book): List<TocEntry> {
    if (book.source != BookSource.EPUB || book.spine.isEmpty()) return emptyList()
    val bookDir = book.bookDir?.let(::File) ?: return book.spine.mapIndexed { index, _ ->
        TocEntry(index, "第 ${index + 1} 章", 0)
    }
    val opfDir = book.opfDir.orEmpty()
    return book.spine.mapIndexed { index, href ->
        val fallback = "第 ${index + 1} 章"
        val chapterFile = if (opfDir.isBlank()) File(bookDir, href) else File(bookDir, "$opfDir/$href")
        val title = if (!chapterFile.exists()) fallback else {
            val raw = runCatching { chapterFile.readText(Charsets.UTF_8) }.getOrNull()
            raw?.let(::extractTitle) ?: fallback
        }
        TocEntry(index, title, 0)
    }
}

private fun extractTitle(raw: String): String? {
    val candidates = listOfNotNull(
        H1_TAG_REGEX.find(raw)?.groupValues?.getOrNull(1),
        H2_TAG_REGEX.find(raw)?.groupValues?.getOrNull(1),
        TITLE_TAG_REGEX.find(raw)?.groupValues?.getOrNull(1),
    )
    for (candidate in candidates) {
        val clean = candidate
            .replace(TAG_STRIP_REGEX, "")
            .replace(WHITESPACE_REGEX, " ")
            .trim()
        if (clean.isNotEmpty()) return clean
    }
    return null
}
