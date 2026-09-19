package com.example.orange.ui.learning

import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.dp
import com.example.orange.data.learning.LearningStatusCounts

val LearningStatusColors = listOf(
    Color(0xFFB0BEC5),
    Color(0xFF42A5F5),
    Color(0xFF66BB6A),
    Color(0xFFFFB74D),
)

@Composable
fun LearningStatusPieChart(
    counts: LearningStatusCounts,
    modifier: Modifier = Modifier,
) {
    val values = listOf(counts.notStarted, counts.learning, counts.learned, counts.mastered)
    val total = values.sum()
    Canvas(modifier = modifier) {
        val stroke = 28.dp.toPx()
        val inset = stroke / 2
        val arcSize = Size(size.width - stroke, size.height - stroke)
        if (total == 0) {
            drawArc(
                color = Color(0xFFE0E0E0),
                startAngle = -90f,
                sweepAngle = 360f,
                useCenter = false,
                topLeft = Offset(inset, inset),
                size = arcSize,
                style = Stroke(stroke, cap = StrokeCap.Butt),
            )
            return@Canvas
        }
        var start = -90f
        values.forEachIndexed { index, value ->
            if (value > 0) {
                val sweep = value.toFloat() / total * 360f
                drawArc(
                    color = LearningStatusColors[index],
                    startAngle = start,
                    sweepAngle = sweep,
                    useCenter = false,
                    topLeft = Offset(inset, inset),
                    size = arcSize,
                    style = Stroke(stroke, cap = StrokeCap.Butt),
                )
                start += sweep
            }
        }
    }
}
