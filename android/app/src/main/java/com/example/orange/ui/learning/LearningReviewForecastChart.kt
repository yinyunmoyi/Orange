package com.example.orange.ui.learning

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.learning.LearningReviewForecast

@Composable
fun LearningReviewForecastChart(
    forecast: LearningReviewForecast,
    modifier: Modifier = Modifier,
) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceVariant,
    ) {
        Column(Modifier.padding(horizontal = 16.dp, vertical = 14.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("复习预览", style = MaterialTheme.typography.titleMedium)
                Text(
                    "未来 ${forecast.days} 天共 ${forecast.total} 词",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontSize = 12.sp,
                )
            }
            if (forecast.items.isEmpty() || forecast.total == 0) {
                Box(
                    modifier = Modifier.fillMaxWidth().height(128.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text("暂无未来复习安排", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                return@Column
            }
            val maxCount = forecast.items.maxOfOrNull { it.count }?.coerceAtLeast(1) ?: 1
            Row(
                modifier = Modifier.fillMaxWidth().height(150.dp).padding(top = 12.dp),
                horizontalArrangement = Arrangement.spacedBy(2.dp),
                verticalAlignment = Alignment.Bottom,
            ) {
                forecast.items.forEachIndexed { index, item ->
                    Column(
                        modifier = Modifier.weight(1f).fillMaxHeight(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        BoxWithConstraints(
                            modifier = Modifier.weight(1f).fillMaxWidth(),
                            contentAlignment = Alignment.BottomCenter,
                        ) {
                            if (item.count > 0) {
                                val labelSpace = 14.dp
                                val barHeight = (maxHeight - labelSpace)
                                    .coerceAtLeast(0.dp) * (item.count.toFloat() / maxCount)
                                Column(
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                ) {
                                    Text(
                                        text = item.count.toString(),
                                        fontSize = 9.sp,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                    Box(
                                        Modifier
                                            .width(10.dp)
                                            .height(barHeight)
                                            .background(
                                                MaterialTheme.colorScheme.primary,
                                                RoundedCornerShape(topStart = 3.dp, topEnd = 3.dp),
                                            )
                                    )
                                }
                            }
                        }
                        Text(
                            text = dateLabel(item.date, index == 0),
                            fontSize = 8.sp,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                        )
                    }
                }
            }
        }
    }
}

private fun dateLabel(date: String, first: Boolean): String {
    val parts = date.split("-")
    if (parts.size != 3) return date
    val month = parts[1].toIntOrNull() ?: return date
    val day = parts[2].toIntOrNull() ?: return date
    return if (first || day == 1) "$month/$day" else day.toString()
}
