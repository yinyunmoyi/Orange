package com.example.orange.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.util.Locale

private fun normalizePosAbbr(raw: String): String {
    return when (raw.trim().lowercase(Locale.US)) {
        "" -> ""
        "n" -> "n."
        "v" -> "v."
        "j", "a", "s" -> "adj."
        "r" -> "adv."
        "p" -> "pron."
        "c" -> "conj."
        "i" -> "prep."
        "u" -> "interj."
        "d" -> "det."
        "m" -> "num."
        "t" -> "to"
        else -> raw
    }
}

private fun extractPrimaryPos(pos: String): String {
    if (pos.isBlank()) return ""
    val first = pos.substringBefore('/')
    return normalizePosAbbr(first.substringBefore(':').trim())
}

private fun displayTag(tag: String): String {
    return when (tag.trim().lowercase(Locale.US)) {
        "zk" -> "初中"
        "gk" -> "高中"
        "ky" -> "考研"
        else -> tag.uppercase(Locale.US)
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun WordMetaChips(
    pos: String,
    oxford: Int,
    tags: List<String>,
    modifier: Modifier = Modifier,
    showPos: Boolean = true,
    labelColor: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    chipBackground: Color = MaterialTheme.colorScheme.surfaceVariant,
    posColor: Color = MaterialTheme.colorScheme.primary,
    oxfordColor: Color = MaterialTheme.colorScheme.tertiary,
) {
    val primaryPos = if (showPos) extractPrimaryPos(pos) else ""
    val cleanedTags = tags.map { it.trim() }.filter { it.isNotEmpty() }
    val showAny = primaryPos.isNotEmpty() || oxford == 1 || cleanedTags.isNotEmpty()
    if (!showAny) return

    FlowRow(
        modifier = modifier,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        if (primaryPos.isNotEmpty()) {
            MetaChip(text = primaryPos, color = posColor)
        }
        if (oxford == 1) {
            MetaChip(text = "牛津", color = oxfordColor)
        }
        cleanedTags.forEach { tag ->
            MetaChip(text = displayTag(tag), color = labelColor)
        }
    }
}

@Composable
private fun MetaChip(text: String, color: Color) {
    Surface(
        shape = RoundedCornerShape(3.dp),
        color = Color.Transparent,
        border = BorderStroke(1.dp, color.copy(alpha = 0.55f)),
    ) {
        Text(
            text = text,
            color = color,
            fontSize = 11.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(PaddingValues(horizontal = 6.dp, vertical = 2.dp)),
        )
    }
}
