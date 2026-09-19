package com.example.orange.ui.learning

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.PlatformTextStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.learning.LearningSessionProgress

private val CompletedColor = Color(0xFF4CAF50)
private val InProgressColor = Color(0xFF42A5F5)
private val NotStartedColor = Color(0xFFE0E0E0)
private val OnLightTextColor = Color(0xFF424242)
private val MinSegmentTextWidth = 24.dp

@Composable
fun LearningSessionProgressBar(
    progress: LearningSessionProgress,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        BoxWithConstraints(
            modifier = Modifier
                .fillMaxWidth()
                .height(18.dp)
                .clip(RoundedCornerShape(9.dp))
                .background(NotStartedColor)
                .semantics {
                    contentDescription = "已完成 ${progress.completed}，背诵中 ${progress.inProgress}，未背诵 ${progress.notStarted}"
                },
        ) {
            val barWidth = maxWidth
            Row(Modifier.fillMaxSize()) {
                if (progress.total > 0) {
                    ProgressSegment(progress.completed, progress.total, barWidth, CompletedColor, Color.White)
                    ProgressSegment(progress.inProgress, progress.total, barWidth, InProgressColor, Color.White)
                    ProgressSegment(progress.notStarted, progress.total, barWidth, NotStartedColor, OnLightTextColor)
                }
            }
        }
        Row(
            modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Legend("已完成 ${progress.completed}", CompletedColor)
            Legend("背诵中 ${progress.inProgress}", InProgressColor)
            Legend("未背诵 ${progress.notStarted}", NotStartedColor)
        }
    }
}

@Composable
private fun RowScope.ProgressSegment(
    value: Int,
    total: Int,
    barWidth: Dp,
    color: Color,
    textColor: Color,
) {
    if (value <= 0) return
    val segmentWidth = barWidth * (value.toFloat() / total.toFloat())
    Box(
        modifier = Modifier.weight(value.toFloat()).fillMaxHeight().background(color),
        contentAlignment = Alignment.Center,
    ) {
        if (segmentWidth >= MinSegmentTextWidth) {
            Text(
                text = value.toString(),
                color = textColor,
                fontSize = 11.sp,
                lineHeight = 11.sp,
                fontWeight = FontWeight.SemiBold,
                style = TextStyle(
                    platformStyle = PlatformTextStyle(includeFontPadding = false),
                ),
            )
        }
    }
}

@Composable
private fun Legend(text: String, color: Color) {
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Box(Modifier.size(8.dp).background(color, CircleShape))
        Text(text = text, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 11.sp)
    }
}
