package com.example.orange.ui.learning

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.snapping.rememberSnapFlingBehavior
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale
import kotlin.math.abs

internal const val UNLIMITED_REVIEW_LIMIT = 9999
internal val DAILY_NEW_OPTIONS: List<Int> = (5..500 step 5).toList()
internal val DAILY_REVIEW_OPTIONS: List<Int> = (5..1000 step 5).toList() + UNLIMITED_REVIEW_LIMIT

internal fun estimatedCompletionDate(
    studyDate: String,
    remainingNewCount: Int,
    dailyNewLimit: Int,
): String {
    require(remainingNewCount >= 0)
    require(dailyNewLimit > 0)
    val format = SimpleDateFormat("yyyy-MM-dd", Locale.US).apply { isLenient = false }
    val date = requireNotNull(format.parse(studyDate)) { "invalid study date" }
    val days = if (remainingNewCount == 0) {
        0
    } else {
        (remainingNewCount + dailyNewLimit - 1) / dailyNewLimit
    }
    return format.format(Calendar.getInstance().apply {
        time = date
        add(Calendar.DAY_OF_MONTH, days)
    }.time)
}

@Composable
internal fun DailyNewLimitDialog(
    currentValue: Int,
    remainingNewCount: Int,
    studyDate: String,
    saving: Boolean,
    error: String?,
    onDismiss: () -> Unit,
    onConfirm: (Int) -> Unit,
) {
    var selected by remember(currentValue) { mutableIntStateOf(currentValue) }
    SettingsPickerDialog(
        title = "设置学习计划",
        saving = saving,
        error = error,
        onDismiss = onDismiss,
        onConfirm = { onConfirm(selected) },
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    "每天学习新词数",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 14.sp,
                    textAlign = TextAlign.Center,
                )
                Spacer(Modifier.height(12.dp))
                WheelPicker(
                    values = DAILY_NEW_OPTIONS,
                    initialValue = currentValue,
                    onValueChange = { selected = it },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    "预计完成时间",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 14.sp,
                    textAlign = TextAlign.Center,
                )
                Spacer(Modifier.height(12.dp))
                Box(
                    modifier = Modifier.height(WheelViewportHeight).fillMaxWidth(),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        estimatedCompletionDate(studyDate, remainingNewCount, selected),
                        fontSize = 18.sp,
                        fontWeight = FontWeight.Medium,
                        textAlign = TextAlign.Center,
                    )
                }
            }
        }
    }
}

@Composable
internal fun DailyReviewLimitDialog(
    currentValue: Int,
    saving: Boolean,
    error: String?,
    onDismiss: () -> Unit,
    onConfirm: (Int) -> Unit,
) {
    var selected by remember(currentValue) { mutableIntStateOf(currentValue) }
    SettingsPickerDialog(
        title = "设置每天复习上限",
        saving = saving,
        error = error,
        onDismiss = onDismiss,
        onConfirm = { onConfirm(selected) },
    ) {
        WheelPicker(
            values = DAILY_REVIEW_OPTIONS,
            initialValue = currentValue,
            onValueChange = { selected = it },
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun SettingsPickerDialog(
    title: String,
    saving: Boolean,
    error: String?,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
    content: @Composable () -> Unit,
) {
    Dialog(onDismissRequest = { if (!saving) onDismiss() }) {
        Surface(
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(8.dp),
            tonalElevation = 6.dp,
        ) {
            Column(modifier = Modifier.padding(top = 24.dp, start = 20.dp, end = 20.dp, bottom = 12.dp)) {
                Text(
                    title,
                    modifier = Modifier.fillMaxWidth(),
                    style = MaterialTheme.typography.titleLarge,
                    textAlign = TextAlign.Center,
                )
                Spacer(Modifier.height(24.dp))
                content()
                if (error != null) {
                    Text(
                        error,
                        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                        color = MaterialTheme.colorScheme.error,
                        fontSize = 12.sp,
                        textAlign = TextAlign.Center,
                    )
                }
                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                    horizontalArrangement = Arrangement.End,
                ) {
                    TextButton(onClick = onDismiss, enabled = !saving) {
                        Text("取消")
                    }
                    Spacer(Modifier.width(16.dp))
                    TextButton(onClick = onConfirm, enabled = !saving) {
                        if (saving) {
                            CircularProgressIndicator(
                                modifier = Modifier.width(18.dp).height(18.dp),
                                strokeWidth = 2.dp,
                            )
                            Spacer(Modifier.width(8.dp))
                        }
                        Text(if (saving) "保存中" else "确定")
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun WheelPicker(
    values: List<Int>,
    initialValue: Int,
    onValueChange: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val initialIndex = values.indexOf(initialValue).takeIf { it >= 0 } ?: 0
    val listState = rememberLazyListState(initialFirstVisibleItemIndex = initialIndex)
    val selectedIndex by remember {
        derivedStateOf {
            val layoutInfo = listState.layoutInfo
            val center = (layoutInfo.viewportStartOffset + layoutInfo.viewportEndOffset) / 2
            layoutInfo.visibleItemsInfo.minByOrNull { item ->
                abs(item.offset + item.size / 2 - center)
            }?.index ?: initialIndex
        }
    }
    LaunchedEffect(selectedIndex) {
        values.getOrNull(selectedIndex)?.let(onValueChange)
    }

    Box(
        modifier = modifier.height(WheelViewportHeight),
        contentAlignment = Alignment.Center,
    ) {
        LazyColumn(
            state = listState,
            flingBehavior = rememberSnapFlingBehavior(lazyListState = listState),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                vertical = WheelItemHeight * 3,
            ),
            modifier = Modifier.fillMaxWidth().height(WheelViewportHeight),
        ) {
            itemsIndexed(values) { index, value ->
                val distance = abs(index - selectedIndex)
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(WheelItemHeight)
                        .graphicsLayer { alpha = when (distance) {
                            0 -> 1f
                            1 -> 0.62f
                            2 -> 0.38f
                            else -> 0.22f
                        } },
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        text = value.toString(),
                        color = MaterialTheme.colorScheme.onSurface,
                        fontSize = if (distance == 0) 20.sp else 17.sp,
                        fontWeight = if (distance == 0) FontWeight.Medium else FontWeight.Normal,
                        textAlign = TextAlign.Center,
                    )
                }
            }
        }
        Column(
            modifier = Modifier.fillMaxWidth().height(WheelItemHeight),
            verticalArrangement = Arrangement.SpaceBetween,
        ) {
            HorizontalDivider(color = MaterialTheme.colorScheme.primary)
            HorizontalDivider(color = MaterialTheme.colorScheme.primary)
        }
    }
}

private val WheelItemHeight = 48.dp
private val WheelViewportHeight = WheelItemHeight * 7
