package com.example.orange.ui.reader

import android.view.GestureDetector
import android.view.MotionEvent
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.windowInsetsTopHeight
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import com.example.orange.R
import com.example.orange.data.Book
import com.example.orange.data.BookRepository
import com.example.orange.data.BookSource
import com.example.orange.data.logging.AppLog
import com.example.orange.data.ReaderPrefs
import com.example.orange.ui.note.NoteEditScreen
import java.io.File

private val BODY_REGEX = Regex("<body[^>]*>([\\s\\S]*?)</body>", RegexOption.IGNORE_CASE)

private enum class ActiveSection { NONE, FONT, THEME }

private data class NoteEditorTarget(
    val itemType: String,
    val text: String,
    val initialNote: String,
    val onSaved: (String) -> Unit,
)

data class ReaderPalette(
    val bg: Color,
    val fg: Color,
    val accent: Color,
    val disabledFg: Color,
)

private fun paletteOf(darkMode: Boolean): ReaderPalette = if (darkMode) {
    ReaderPalette(
        bg = Color(0xFF1F1F1F),
        fg = Color(0xFFDCDCDC),
        accent = Color(0xFF8AB4F8),
        disabledFg = Color(0xFF666666),
    )
} else {
    ReaderPalette(
        bg = Color(0xFFF5EFE0),
        fg = Color(0xFF222222),
        accent = Color(0xFFC96A2C),
        disabledFg = Color(0xFF8B8B8B),
    )
}

@Composable
fun ReaderScreen(
    book: Book,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val isEpub = book.source == BookSource.EPUB && book.spine.isNotEmpty()
    val total = if (isEpub) book.spine.size else 1

    val savedProgress = remember(book.id) { BookRepository.progressOf(book.id) }
    val initialPrefs = remember { BookRepository.getReaderPrefs() }

    var chapterIndex by rememberSaveable(book.id) {
        val initial = if (isEpub) {
            (savedProgress?.chapterIndex ?: 0).coerceIn(0, book.spine.size - 1)
        } else {
            0
        }
        mutableIntStateOf(initial)
    }
    if (chapterIndex >= total) chapterIndex = 0

    var pendingFraction by remember(book.id) {
        mutableStateOf(if (isEpub) savedProgress?.fraction ?: 0f else 0f)
    }

    var fontSize by rememberSaveable { mutableIntStateOf(initialPrefs.fontSize) }
    var darkMode by rememberSaveable { mutableStateOf(initialPrefs.darkMode) }

    val palette = paletteOf(darkMode)

    val ctx = LocalContext.current
    DisposableEffect(darkMode) {
        val activity = ctx as? android.app.Activity
        val window = activity?.window
        val decor = window?.decorView
        if (window != null && decor != null) {
            val controller = WindowCompat.getInsetsController(window, decor)
            controller.isAppearanceLightStatusBars = !darkMode
            controller.isAppearanceLightNavigationBars = !darkMode
        }
        onDispose {
            if (window != null && decor != null) {
                val controller = WindowCompat.getInsetsController(window, decor)
                controller.isAppearanceLightStatusBars = true
                controller.isAppearanceLightNavigationBars = true
            }
        }
    }

    var showToolbar by remember { mutableStateOf(false) }
    var showToc by rememberSaveable(book.id) { mutableStateOf(false) }
    var activeSection by remember { mutableStateOf(ActiveSection.NONE) }
    var atBottom by remember { mutableStateOf(false) }
    var remainingPages by remember { mutableIntStateOf(0) }
    var chapterPercent by remember { mutableStateOf(0f) }
    var lookupState by remember { mutableStateOf<LookupUiState?>(null) }
    var sentenceToAnalyze by rememberSaveable(book.id) { mutableStateOf<String?>(null) }
    var noteEditor by remember { mutableStateOf<NoteEditorTarget?>(null) }

    LaunchedEffect(book.id, chapterIndex) {
        atBottom = false
    }

    var webViewRef by remember { mutableStateOf<WebView?>(null) }

    val hasPrev = isEpub && chapterIndex > 0
    val hasNext = isEpub && chapterIndex < total - 1

    val (html, baseUrl) = remember(book.id, chapterIndex, fontSize, darkMode) {
        loadChapterHtml(book, chapterIndex, fontSize, darkMode)
    }

    fun currentFraction(): Float {
        val wv = webViewRef ?: return 0f
        val denom = (wv.contentHeight * wv.scale).toInt() - wv.height
        if (denom <= 0) return 0f
        return (wv.scrollY.toFloat() / denom.toFloat()).coerceIn(0f, 1f)
    }

    fun updateChapterProgress(view: WebView) {
        val progress = computeChapterProgress(view)
        remainingPages = progress.remainingPages
        chapterPercent = progress.percent
    }

    fun saveCurrentProgress(index: Int) {
        BookRepository.saveProgress(book.id, index, currentFraction())
    }

    fun applyPref(mutator: () -> Unit) {
        if (isEpub) pendingFraction = currentFraction()
        mutator()
        BookRepository.saveReaderPrefs(ReaderPrefs(fontSize, darkMode))
    }

    val currentIndexState = rememberUpdatedState(chapterIndex)
    val isEpubState = rememberUpdatedState(isEpub)
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner, book.id) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_PAUSE && isEpubState.value) {
                saveCurrentProgress(currentIndexState.value)
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    val exitReader: () -> Unit = {
        if (isEpub) saveCurrentProgress(chapterIndex)
        onBack()
    }

    val handleBack: () -> Unit = {
        when {
            noteEditor != null -> noteEditor = null
            sentenceToAnalyze != null -> sentenceToAnalyze = null
            lookupState != null -> lookupState = null
            showToc -> showToc = false
            activeSection != ActiveSection.NONE -> activeSection = ActiveSection.NONE
            showToolbar -> showToolbar = false
            else -> exitReader()
        }
    }

    BackHandler(onBack = handleBack)

    val onSelectionLookup by rememberUpdatedState<(
        String,
        android.graphics.Rect,
        String,
        Int,
        Int,
        String,
        Int,
        Int,
    ) -> Unit> { word, rect, context, wordStart, wordEnd, paragraph, selectionStart, selectionEnd ->
        val isPhrase = word.any { it.isWhitespace() }
        lookupState = LookupUiState(
            word = word,
            anchorRectPx = rect,
            context = context,
            wordStart = wordStart,
            wordEnd = wordEnd,
            paragraph = paragraph,
            selectionStart = selectionStart,
            selectionEnd = selectionEnd,
            isPhrase = isPhrase,
        )
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(palette.bg),
    ) {
        if (showToc) {
            TocScreen(
                book = book,
                currentIndex = chapterIndex,
                palette = palette,
                onBack = { showToc = false },
                onChapterSelected = { targetIndex ->
                    if (targetIndex != chapterIndex) {
                        saveCurrentProgress(chapterIndex)
                        pendingFraction = 0f
                        chapterIndex = targetIndex
                    }
                    showToc = false
                    showToolbar = false
                    activeSection = ActiveSection.NONE
                },
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { context ->
                    val gestureDetector = GestureDetector(
                        context,
                        object : GestureDetector.SimpleOnGestureListener() {
                            override fun onSingleTapUp(e: MotionEvent): Boolean {
                                if (showToolbar) {
                                    showToolbar = false
                                    activeSection = ActiveSection.NONE
                                } else {
                                    showToolbar = true
                                }
                                return false
                            }
                        },
                    )
                    LookupWebView(context).apply {
                        settings.allowFileAccess = true
                        settings.javaScriptEnabled = true
                        onLookupSelection = {
                                word,
                                rect,
                                ctxText,
                                wordStart,
                                wordEnd,
                                paragraph,
                                selectionStart,
                                selectionEnd,
                            ->
                            onSelectionLookup(
                                word,
                                rect,
                                ctxText,
                                wordStart,
                                wordEnd,
                                paragraph,
                                selectionStart,
                                selectionEnd,
                            )
                        }
                        onAnalyzeSelection = { s ->
                            AppLog.d("ReaderScreen", "onAnalyzeSelection received len=${s.length} text=${s.take(120)}")
                            sentenceToAnalyze = s
                        }
                        onCopySelection = {
                            android.widget.Toast.makeText(context, "已复制", android.widget.Toast.LENGTH_SHORT).show()
                        }
                        webViewClient = object : WebViewClient() {
                            override fun onPageFinished(view: WebView, url: String?) {
                                val target = pendingFraction
                                if (target <= 0f) {
                                    view.scrollTo(0, 0)
                                    pendingFraction = 0f
                                    view.post {
                                        atBottom = isAtBottom(view)
                                        updateChapterProgress(view)
                                    }
                                    view.postDelayed({
                                        atBottom = isAtBottom(view)
                                        updateChapterProgress(view)
                                    }, 150)
                                    return
                                }
                                view.post {
                                    restoreScroll(view, target, retry = true)
                                    atBottom = isAtBottom(view)
                                    updateChapterProgress(view)
                                }
                                view.postDelayed({
                                    atBottom = isAtBottom(view)
                                    updateChapterProgress(view)
                                }, 150)
                                pendingFraction = 0f
                            }
                        }
                        addOnLayoutChangeListener { v, _, _, _, _, _, _, _, _ ->
                            val wv = v as WebView
                            atBottom = isAtBottom(wv)
                            updateChapterProgress(wv)
                        }
                        setOnScrollChangeListener { v, _, _, _, _ ->
                            val wv = v as WebView
                            atBottom = isAtBottom(wv)
                            updateChapterProgress(wv)
                        }
                        setOnTouchListener { _, e ->
                            gestureDetector.onTouchEvent(e)
                            false
                        }
                        webViewRef = this
                    }
                },
                update = { webView ->
                    webView.loadDataWithBaseURL(baseUrl, html, "text/html", "utf-8", null)
                },
            )

            AnimatedVisibility(
                visible = showToolbar,
                enter = fadeIn() + slideInVertically { -it },
                exit = fadeOut() + slideOutVertically { -it },
                modifier = Modifier.align(Alignment.TopCenter).statusBarsPadding(),
            ) {
                ReaderTopBar(
                    palette = palette,
                    title = if (isEpub) "${book.title} · 第 ${chapterIndex + 1}/${total} 章" else book.title,
                    onBack = exitReader,
                )
            }

            Column(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth(),
            ) {
                AnimatedVisibility(
                    visible = atBottom && isEpub,
                    enter = fadeIn() + slideInVertically { it },
                    exit = fadeOut() + slideOutVertically { it },
                ) {
                    ChapterNavPanel(
                        palette = palette,
                        current = chapterIndex + 1,
                        total = total,
                        hasPrev = hasPrev,
                        hasNext = hasNext,
                        onPrev = {
                            if (hasPrev) {
                                saveCurrentProgress(chapterIndex)
                                pendingFraction = 0f
                                chapterIndex -= 1
                            }
                        },
                        onNext = {
                            if (hasNext) {
                                saveCurrentProgress(chapterIndex)
                                pendingFraction = 0f
                                chapterIndex += 1
                            }
                        },
                    )
                }
                AnimatedVisibility(
                    visible = showToolbar && activeSection == ActiveSection.FONT,
                    enter = fadeIn() + slideInVertically { it },
                    exit = fadeOut() + slideOutVertically { it },
                ) {
                    FontPanel(
                        palette = palette,
                        fontSize = fontSize,
                        onDecrease = {
                            if (fontSize > BookRepository.MIN_FONT_SIZE) {
                                applyPref { fontSize = (fontSize - 2).coerceAtLeast(BookRepository.MIN_FONT_SIZE) }
                            }
                        },
                        onIncrease = {
                            if (fontSize < BookRepository.MAX_FONT_SIZE) {
                                applyPref { fontSize = (fontSize + 2).coerceAtMost(BookRepository.MAX_FONT_SIZE) }
                            }
                        },
                    )
                }
                AnimatedVisibility(
                    visible = showToolbar && activeSection == ActiveSection.THEME,
                    enter = fadeIn() + slideInVertically { it },
                    exit = fadeOut() + slideOutVertically { it },
                ) {
                    ThemePanel(
                        palette = palette,
                        darkMode = darkMode,
                        onSelect = { targetDark ->
                            if (darkMode != targetDark) {
                                applyPref { darkMode = targetDark }
                            }
                        },
                    )
                }
                AnimatedVisibility(
                    visible = showToolbar,
                    enter = fadeIn() + slideInVertically { it },
                    exit = fadeOut() + slideOutVertically { it },
                ) {
                    ReaderIconBar(
                        palette = palette,
                        activeSection = activeSection,
                        onToc = {
                            if (isEpub) {
                                pendingFraction = currentFraction()
                                saveCurrentProgress(chapterIndex)
                            }
                            showToc = true
                            showToolbar = false
                            activeSection = ActiveSection.NONE
                        },
                        onFont = {
                            activeSection = if (activeSection == ActiveSection.FONT) {
                                ActiveSection.NONE
                            } else {
                                ActiveSection.FONT
                            }
                        },
                        onTheme = {
                            activeSection = if (activeSection == ActiveSection.THEME) {
                                ActiveSection.NONE
                            } else {
                                ActiveSection.THEME
                            }
                        },
                    )
                }
            }

            AnimatedVisibility(
                visible = isEpub && !showToolbar,
                enter = fadeIn(),
                exit = fadeOut(),
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .navigationBarsPadding()
                    .padding(end = 12.dp, bottom = 12.dp),
            ) {
                Box(
                    modifier = Modifier
                        .background(Color(0xCC000000), RoundedCornerShape(8.dp))
                        .padding(horizontal = 10.dp, vertical = 6.dp),
                ) {
                    Text(
                        text = "本章剩余${remainingPages}页 ${"%.1f".format(chapterPercent)}%",
                        color = Color.White,
                        fontSize = 12.sp,
                    )
                }
            }
        }

        Box(
            modifier = Modifier
                .align(Alignment.TopCenter)
                .fillMaxWidth()
                .windowInsetsTopHeight(WindowInsets.statusBars)
                .background(if (darkMode) Color.Black else Color.White),
        )

        lookupState?.let { s ->
            LookupPopup(
                state = s,
                palette = palette,
                onDismiss = { lookupState = null },
                onEditNote = { itemType, text, note, onSaved ->
                    noteEditor = NoteEditorTarget(itemType, text, note, onSaved)
                },
            )
        }

        sentenceToAnalyze?.let { s ->
            SentenceAnalyzeScreen(
                sentence = s,
                palette = palette,
                darkMode = darkMode,
                onBack = { sentenceToAnalyze = null },
            )
        }

        noteEditor?.let { target ->
            NoteEditScreen(
                itemType = target.itemType,
                text = target.text,
                initialNote = target.initialNote,
                onBack = { noteEditor = null },
                onSaved = { saved ->
                    target.onSaved(saved)
                    noteEditor = null
                },
                modifier = Modifier.fillMaxSize(),
            )
        }
    }
}

private fun isAtBottom(view: WebView): Boolean {
    val max = (view.contentHeight * view.scale).toInt() - view.height
    return max <= 0 || view.scrollY >= max - 4
}

private data class ChapterProgress(val remainingPages: Int, val percent: Float)

private fun computeChapterProgress(view: WebView): ChapterProgress {
    val viewport = view.height.toFloat()
    val total = view.contentHeight * view.scale
    if (viewport <= 0f || total <= 0f) return ChapterProgress(0, 0f)
    val scrollY = view.scrollY.toFloat()
    val remainingPx = (total - scrollY - viewport).coerceAtLeast(0f)
    val pages = kotlin.math.ceil(remainingPx / viewport).toInt()
    val read = (scrollY + viewport).coerceAtMost(total)
    val percent = (read / total * 100f).coerceIn(0f, 100f)
    return ChapterProgress(pages, percent)
}

private fun restoreScroll(view: WebView, fraction: Float, retry: Boolean) {
    val denom = (view.contentHeight * view.scale).toInt() - view.height
    if (denom > 0) {
        view.scrollTo(0, (denom * fraction).toInt())
    } else if (retry) {
        view.postDelayed({ restoreScroll(view, fraction, retry = false) }, 80)
    }
}

@Composable
private fun ReaderTopBar(
    palette: ReaderPalette,
    title: String,
    onBack: () -> Unit,
) {
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
            text = title,
            color = palette.fg,
            fontSize = 16.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier
                .align(Alignment.Center)
                .padding(horizontal = 72.dp),
        )
    }
}

@Composable
private fun ChapterNavPanel(
    palette: ReaderPalette,
    current: Int,
    total: Int,
    hasPrev: Boolean,
    hasNext: Boolean,
    onPrev: () -> Unit,
    onNext: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(palette.bg)
            .padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        NavTextButton(
            text = "上一章",
            enabled = hasPrev,
            palette = palette,
            onClick = onPrev,
        )
        Text(text = "$current / $total", color = palette.fg, fontSize = 14.sp)
        NavTextButton(
            text = "下一章",
            enabled = hasNext,
            palette = palette,
            onClick = onNext,
        )
    }
}

@Composable
private fun NavTextButton(
    text: String,
    enabled: Boolean,
    palette: ReaderPalette,
    onClick: () -> Unit,
) {
    val color = if (enabled) palette.fg else palette.disabledFg
    Text(
        text = text,
        color = color,
        fontSize = 15.sp,
        modifier = Modifier
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 6.dp),
    )
}

@Composable
private fun ReaderIconBar(
    palette: ReaderPalette,
    activeSection: ActiveSection,
    onToc: () -> Unit,
    onFont: () -> Unit,
    onTheme: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(palette.bg)
            .navigationBarsPadding()
            .padding(horizontal = 24.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconBarButton(
            iconResId = R.drawable.ic_toc,
            highlighted = false,
            palette = palette,
            onClick = onToc,
            description = "目录",
        )
        IconBarButton(
            iconResId = R.drawable.ic_font_size,
            highlighted = activeSection == ActiveSection.FONT,
            palette = palette,
            onClick = onFont,
            description = "字体",
        )
        IconBarButton(
            iconResId = R.drawable.ic_theme_mode,
            highlighted = activeSection == ActiveSection.THEME,
            palette = palette,
            onClick = onTheme,
            description = "主题",
        )
    }
}

@Composable
private fun IconBarButton(
    iconResId: Int,
    highlighted: Boolean,
    palette: ReaderPalette,
    onClick: () -> Unit,
    description: String,
) {
    val tint = if (highlighted) palette.accent else palette.fg
    IconButton(onClick = onClick) {
        Icon(
            painter = painterResource(id = iconResId),
            contentDescription = description,
            tint = tint,
            modifier = Modifier.size(24.dp),
        )
    }
}

@Composable
private fun FontPanel(
    palette: ReaderPalette,
    fontSize: Int,
    onDecrease: () -> Unit,
    onIncrease: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(palette.bg)
            .padding(horizontal = 24.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val decEnabled = fontSize > BookRepository.MIN_FONT_SIZE
        val incEnabled = fontSize < BookRepository.MAX_FONT_SIZE
        Text(
            text = "A-",
            color = if (decEnabled) palette.fg else palette.disabledFg,
            fontSize = 18.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier
                .clickable(enabled = decEnabled, onClick = onDecrease)
                .padding(horizontal = 12.dp, vertical = 6.dp),
        )
        Text(
            text = "$fontSize",
            color = palette.fg,
            fontSize = 16.sp,
        )
        Text(
            text = "A+",
            color = if (incEnabled) palette.fg else palette.disabledFg,
            fontSize = 18.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier
                .clickable(enabled = incEnabled, onClick = onIncrease)
                .padding(horizontal = 12.dp, vertical = 6.dp),
        )
    }
}

@Composable
private fun ThemePanel(
    palette: ReaderPalette,
    darkMode: Boolean,
    onSelect: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(palette.bg)
            .padding(horizontal = 24.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = "日间",
            color = if (!darkMode) palette.accent else palette.fg,
            fontSize = 16.sp,
            fontWeight = if (!darkMode) FontWeight.SemiBold else FontWeight.Normal,
            modifier = Modifier
                .clickable { onSelect(false) }
                .padding(horizontal = 20.dp, vertical = 6.dp),
        )
        Text(
            text = "夜间",
            color = if (darkMode) palette.accent else palette.fg,
            fontSize = 16.sp,
            fontWeight = if (darkMode) FontWeight.SemiBold else FontWeight.Normal,
            modifier = Modifier
                .clickable { onSelect(true) }
                .padding(horizontal = 20.dp, vertical = 6.dp),
        )
    }
}

private fun loadChapterHtml(
    book: Book,
    index: Int,
    fontSize: Int,
    darkMode: Boolean,
): Pair<String, String?> {
    if (book.source != BookSource.EPUB || book.spine.isEmpty()) {
        return wrapHtml("<p>该书籍暂不可读</p>", fontSize, darkMode) to null
    }
    val bookDir = book.bookDir?.let(::File)
        ?: return wrapHtml("<p>该书籍暂不可读</p>", fontSize, darkMode) to null
    val opfDir = book.opfDir.orEmpty()
    val href = book.spine.getOrNull(index)
        ?: return wrapHtml("<p>该书籍暂不可读</p>", fontSize, darkMode) to null
    val chapterFile = if (opfDir.isBlank()) File(bookDir, href) else File(bookDir, "$opfDir/$href")
    if (!chapterFile.exists()) {
        return wrapHtml("<p>章节文件不存在：$href</p>", fontSize, darkMode) to null
    }
    val chapterDir = chapterFile.parentFile ?: bookDir
    val raw = runCatching { chapterFile.readText(Charsets.UTF_8) }
        .getOrElse { return wrapHtml("<p>章节读取失败：${it.message}</p>", fontSize, darkMode) to null }
    val bodyInner = BODY_REGEX.find(raw)?.groupValues?.getOrNull(1) ?: raw
    val html = wrapHtml(bodyInner, fontSize, darkMode)
    val baseUrl = "file://${chapterDir.absolutePath}/"
    return html to baseUrl
}

private fun wrapHtml(bodyInner: String, fontSize: Int, darkMode: Boolean): String {
    val bg = if (darkMode) "#181818" else "#fbf7ef"
    val fg = if (darkMode) "#dcdcdc" else "#222"
    val css = """
      body {
        margin: 0;
        padding: 24px 24px 140px 24px;
        font-family: -apple-system, "PingFang SC", "Noto Sans CJK SC", sans-serif;
        color: $fg;
        background: $bg;
        line-height: 1.8;
        font-size: ${fontSize}px;
        word-wrap: break-word;
      }
      h1, h2, h3 { line-height: 1.4; }
      p { margin: 12px 0; }
      img { max-width: 100%; height: auto; display: block; margin: 12px auto; }
      a { color: ${if (darkMode) "#8ab4f8" else "#1a73e8"}; }
    """.trimIndent()
    return """
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>$css</style>
</head>
<body>
$bodyInner
</body>
</html>
""".trimIndent()
}
