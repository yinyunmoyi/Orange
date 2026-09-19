package com.example.orange.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

val WordLevelBadgeSize: Dp = 16.dp

internal data class WordLevelBadgeColors(
    val background: Color,
    val content: Color,
)

internal fun wordLevelBadgeColors(level: Int): WordLevelBadgeColors = when (level) {
    in 1..9 -> WordLevelBadgeColors(
        background = Color(0xFF2E7D32),
        content = Color.White,
    )
    in 10..19 -> WordLevelBadgeColors(
        background = Color(0xFF1565C0),
        content = Color.White,
    )
    in 20..29 -> WordLevelBadgeColors(
        background = Color(0xFFF9A825),
        content = Color(0xFF1C1B1F),
    )
    in 30..39 -> WordLevelBadgeColors(
        background = Color(0xFFC62828),
        content = Color.White,
    )
    else -> WordLevelBadgeColors(
        background = Color.Black,
        content = Color.White,
    )
}

@Composable
fun WordLevelBadge(
    level: Int,
    modifier: Modifier = Modifier,
) {
    if (level < 1) return
    val colors = wordLevelBadgeColors(level)
    val fontSize = when (level.toString().length) {
        1, 2 -> 8.sp
        3 -> 6.sp
        4 -> 5.sp
        else -> 4.sp
    }
    Surface(
        modifier = modifier
            .size(WordLevelBadgeSize)
            .clearAndSetSemantics {
                contentDescription = "等级 $level"
            },
        shape = CircleShape,
        color = colors.background,
        contentColor = colors.content,
    ) {
        Box(
            modifier = Modifier.fillMaxSize(),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = level.toString(),
                style = MaterialTheme.typography.labelSmall.copy(
                    fontSize = fontSize,
                    lineHeight = fontSize,
                    fontWeight = FontWeight.Bold,
                ),
                maxLines = 1,
            )
        }
    }
}
