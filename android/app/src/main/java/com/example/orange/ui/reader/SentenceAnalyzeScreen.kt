package com.example.orange.ui.reader

import com.example.orange.data.logging.AppLog as Log
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.clickable
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.word.SentenceAnalysis
import com.example.orange.data.word.SentenceChunk
import com.example.orange.data.word.WordApi
import com.example.orange.ui.components.LoadingDots
import kotlinx.coroutines.launch

private sealed class TranslationState {
    object Loading : TranslationState()
    data class Streaming(val text: String) : TranslationState()
    data class Success(val text: String) : TranslationState()
    data class Failure(val message: String) : TranslationState()
}

@Composable
fun SentenceAnalyzeScreen(
    sentence: String,
    palette: ReaderPalette,
    darkMode: Boolean,
    onBack: () -> Unit,
) {
    var analyzeState by remember(sentence) { mutableStateOf<LoadState<SentenceAnalysis>>(LoadState.Loading) }
    var translation by remember(sentence) { mutableStateOf<TranslationState>(TranslationState.Loading) }
    var favoriteStatusLoading by remember(sentence) { mutableStateOf(true) }
    var favorited by remember(sentence) { mutableStateOf(false) }
    var favoriting by remember(sentence) { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    LaunchedEffect(sentence) {
        launch {
            analyzeState = WordApi.analyzeSentence(sentence).fold(
                onSuccess = { LoadState.Success(it) },
                onFailure = {
                    Log.w(
                        "SentenceAnalyze",
                        "analyze failed sentence=${sentence.take(80)} err_type=${it.javaClass.name} err_msg=${it.message}",
                        it,
                    )
                    LoadState.Failure("加载失败")
                },
            )
        }
        launch {
            val buf = StringBuilder()
            WordApi.translateSentenceStream(
                sentence = sentence,
                onDelta = { d ->
                    buf.append(d)
                    translation = TranslationState.Streaming(buf.toString())
                },
                onDone = {
                    val txt = buf.toString()
                    translation = if (txt.isBlank()) {
                        TranslationState.Failure("加载失败")
                    } else {
                        TranslationState.Success(txt)
                    }
                },
                onError = { msg ->
                    Log.w("SentenceAnalyze", "translate failed sentence=${sentence.take(80)} msg=$msg")
                    translation = TranslationState.Failure("加载失败")
                },
            )
        }
        launch {
            favoriteStatusLoading = true
            WordApi.sentenceFavoriteStatus(sentence).fold(
                onSuccess = {
                    favorited = it.favorited
                    favoriteStatusLoading = false
                },
                onFailure = {
                    favoriteStatusLoading = false
                    snackbarHostState.showSnackbar("收藏状态加载失败")
                },
            )
        }
    }

    Surface(
        color = palette.bg,
        contentColor = palette.fg,
        modifier = Modifier.fillMaxSize(),
    ) {
        Box(modifier = Modifier.fillMaxSize()) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .statusBarsPadding()
                    .navigationBarsPadding(),
            ) {
                val completedTranslation = (translation as? TranslationState.Success)?.text.orEmpty()
                val canFavorite = completedTranslation.isNotBlank() &&
                    !favoriteStatusLoading &&
                    !favorited &&
                    !favoriting
                SentenceTopBar(
                    palette = palette,
                    favorited = favorited,
                    favoriteEnabled = canFavorite,
                    onFavorite = {
                        if (canFavorite) {
                            favoriting = true
                            scope.launch {
                                WordApi.favoriteSentence(sentence, completedTranslation).fold(
                                    onSuccess = {
                                        favorited = true
                                        favoriting = false
                                    },
                                    onFailure = {
                                        favoriting = false
                                        snackbarHostState.showSnackbar("收藏失败，请重试")
                                    },
                                )
                            }
                        }
                    },
                    onBack = onBack,
                )
                HorizontalDivider(color = palette.disabledFg.copy(alpha = 0.3f))
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                ) {
                    SentenceOriginalArea(
                        sentence = sentence,
                        analyzeState = analyzeState,
                        palette = palette,
                        darkMode = darkMode,
                    )
                    Spacer(Modifier.height(16.dp))
                    SectionTitle(text = "翻译", palette = palette)
                    Spacer(Modifier.height(6.dp))
                    TranslationArea(translation = translation, palette = palette)
                    Spacer(Modifier.height(20.dp))
                    SectionTitle(text = "语法解析", palette = palette)
                    Spacer(Modifier.height(6.dp))
                    AnalyzeArea(
                        analyzeState = analyzeState,
                        palette = palette,
                        darkMode = darkMode,
                    )
                    Spacer(Modifier.height(16.dp))
                }
            }
            SnackbarHost(
                hostState = snackbarHostState,
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .navigationBarsPadding(),
            )
        }
    }
}

@Composable
private fun SentenceTopBar(
    palette: ReaderPalette,
    favorited: Boolean,
    favoriteEnabled: Boolean,
    onFavorite: () -> Unit,
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
            text = "句子分析",
            color = palette.fg,
            fontSize = 16.sp,
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier
                .align(Alignment.Center)
                .padding(horizontal = 72.dp),
        )
        IconButton(
            onClick = onFavorite,
            enabled = favoriteEnabled,
            modifier = Modifier.align(Alignment.CenterEnd),
        ) {
            Icon(
                painter = painterResource(
                    if (favorited) R.drawable.ic_star else R.drawable.ic_star_outline,
                ),
                contentDescription = if (favorited) "已收藏" else "收藏句子",
                tint = if (favorited) palette.accent else palette.fg,
            )
        }
    }
}

@Composable
private fun SentenceOriginalArea(
    sentence: String,
    analyzeState: LoadState<SentenceAnalysis>,
    palette: ReaderPalette,
    darkMode: Boolean,
) {
    val annotated: AnnotatedString = when (analyzeState) {
        is LoadState.Success -> buildColorizedSentence(sentence, analyzeState.data.chunks, palette.fg, darkMode)
        else -> AnnotatedString(sentence)
    }
    Text(
        text = annotated,
        color = palette.fg,
        fontSize = 18.sp,
        lineHeight = 28.sp,
        modifier = Modifier.fillMaxWidth(),
    )
}

private fun buildColorizedSentence(
    sentence: String,
    chunks: List<SentenceChunk>,
    fallback: Color,
    darkMode: Boolean,
): AnnotatedString = buildAnnotatedString {
    if (chunks.isEmpty()) {
        append(sentence)
        return@buildAnnotatedString
    }
    val joined = buildString {
        for (c in chunks) append(c.text)
    }
    if (joined == sentence) {
        for (chunk in chunks) {
            withStyle(SpanStyle(color = chunkColorFor(chunk.type, darkMode))) { append(chunk.text) }
        }
        return@buildAnnotatedString
    }
    Log.w("SentenceAnalyze", "chunks do not match sentence exactly, fallback to indexOf alignment. sentence_len=${sentence.length} joined_len=${joined.length}")
    var cursor = 0
    var failed = false
    for (chunk in chunks) {
        val text = chunk.text
        if (text.isEmpty()) continue
        val idx = sentence.indexOf(text, startIndex = cursor)
        if (idx < 0) {
            Log.w("SentenceAnalyze", "chunk not found in remaining sentence, stopping colorization. text=${text.take(60)} cursor=$cursor remaining=${sentence.substring(cursor).take(60)}")
            withStyle(SpanStyle(color = fallback)) { append(sentence.substring(cursor)) }
            failed = true
            break
        }
        if (idx > cursor) {
            withStyle(SpanStyle(color = fallback)) { append(sentence.substring(cursor, idx)) }
        }
        withStyle(SpanStyle(color = chunkColorFor(chunk.type, darkMode))) { append(text) }
        cursor = idx + text.length
    }
    if (!failed && cursor < sentence.length) {
        withStyle(SpanStyle(color = fallback)) { append(sentence.substring(cursor)) }
    }
}

@Composable
private fun SectionTitle(text: String, palette: ReaderPalette) {
    Text(
        text = text,
        color = palette.fg,
        fontSize = 14.sp,
        fontWeight = FontWeight.SemiBold,
    )
}

@Composable
private fun TranslationArea(translation: TranslationState, palette: ReaderPalette) {
    when (translation) {
        TranslationState.Loading -> LoadingDots(color = palette.fg)
        is TranslationState.Streaming -> Text(
            text = translation.text,
            color = palette.fg,
            fontSize = 15.sp,
            lineHeight = 24.sp,
        )
        is TranslationState.Success -> Text(
            text = translation.text,
            color = palette.fg,
            fontSize = 15.sp,
            lineHeight = 24.sp,
        )
        is TranslationState.Failure -> Text(
            text = translation.message,
            color = palette.disabledFg,
            fontSize = 14.sp,
        )
    }
}

@Composable
private fun AnalyzeArea(
    analyzeState: LoadState<SentenceAnalysis>,
    palette: ReaderPalette,
    darkMode: Boolean,
) {
    when (analyzeState) {
        LoadState.Loading -> LoadingDots(color = palette.fg)
        is LoadState.Failure -> Text(
            text = analyzeState.message,
            color = palette.disabledFg,
            fontSize = 14.sp,
        )
        is LoadState.Success -> {
            val chunks = analyzeState.data.chunks
            val structure = analyzeState.data.structure.trim()
            if (chunks.isEmpty() && structure.isEmpty()) {
                Text(text = "暂无内容", color = palette.disabledFg, fontSize = 13.sp)
            } else {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    if (structure.isNotEmpty()) {
                        StructureSummary(text = structure, palette = palette)
                    }
                    for (chunk in chunks) {
                        ChunkCard(chunk = chunk, palette = palette, darkMode = darkMode)
                    }
                }
            }
        }
    }
}

@Composable
private fun StructureSummary(text: String, palette: ReaderPalette) {
    Box(
        modifier = Modifier
            .fillMaxWidth()
            .background(
                color = palette.disabledFg.copy(alpha = 0.10f),
                shape = RoundedCornerShape(6.dp),
            )
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Text(
            text = text,
            color = palette.fg,
            fontSize = 14.sp,
            lineHeight = 22.sp,
        )
    }
}

@Composable
private fun ChunkCard(chunk: SentenceChunk, palette: ReaderPalette, darkMode: Boolean) {
    val color = chunkColorFor(chunk.type, darkMode)
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                modifier = Modifier
                    .background(color = color, shape = RoundedCornerShape(4.dp))
                    .padding(horizontal = 8.dp, vertical = 2.dp)
                    .widthIn(min = 40.dp),
            ) {
                Text(
                    text = chunkTypeLabel(chunk.type),
                    color = Color.White,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                )
            }
            Spacer(Modifier.width(8.dp))
            if (chunk.role.isNotBlank()) {
                Text(
                    text = chunk.role,
                    color = palette.disabledFg,
                    fontSize = 12.sp,
                )
            }
        }
        Spacer(Modifier.height(4.dp))
        Text(
            text = chunk.text,
            color = color,
            fontSize = 15.sp,
            lineHeight = 22.sp,
        )
        if (chunk.translation.isNotBlank()) {
            Spacer(Modifier.height(2.dp))
            Text(
                text = chunk.translation,
                color = palette.fg,
                fontSize = 14.sp,
                lineHeight = 22.sp,
            )
        }
    }
}

private fun chunkTypeLabel(type: String): String = when (type) {
    "main" -> "主干"
    "modifier" -> "修饰"
    "adverbial" -> "状语"
    "parenthetical" -> "插入语"
    "participle" -> "分词"
    "quote" -> "引语"
    "other" -> "其它"
    else -> type.ifBlank { "其它" }
}

private fun chunkColorFor(type: String, darkMode: Boolean): Color = if (darkMode) {
    when (type) {
        "main" -> Color(0xFF8AB4F8)
        "modifier" -> Color(0xFF81C784)
        "adverbial" -> Color(0xFFF48FB1)
        "parenthetical" -> Color(0xFFCE93D8)
        "participle" -> Color(0xFF80DEEA)
        "quote" -> Color(0xFFFFB74D)
        else -> Color(0xFFBCAAA4)
    }
} else {
    when (type) {
        "main" -> Color(0xFFC96A2C)
        "modifier" -> Color(0xFF2E7D32)
        "adverbial" -> Color(0xFFAD1457)
        "parenthetical" -> Color(0xFF8E24AA)
        "participle" -> Color(0xFF00838F)
        "quote" -> Color(0xFFEF6C00)
        else -> Color(0xFF5D4037)
    }
}
