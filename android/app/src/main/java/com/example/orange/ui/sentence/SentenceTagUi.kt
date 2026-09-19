package com.example.orange.ui.sentence

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.data.word.SentenceFavoriteTag
import com.example.orange.data.word.SentenceTag

internal val SentenceTagColorPalette = listOf(
    "#D32F2F",
    "#E65100",
    "#F9A825",
    "#2E7D32",
    "#00897B",
    "#1565C0",
    "#5E35B1",
    "#C2185B",
)

internal fun parseSentenceTagArgb(value: String): Long? {
    if (!Regex("^#[0-9A-Fa-f]{6}$").matches(value)) return null
    return runCatching {
        0xFF000000L or value.drop(1).toLong(16)
    }.getOrNull()
}

internal fun toggleSentenceTagSelection(
    selected: Set<Long>,
    tagId: Long,
): Set<Long> {
    if (tagId <= 0L) return selected
    return if (tagId in selected) selected - tagId else selected + tagId
}

internal fun sentenceTagColor(value: String): Color =
    Color(parseSentenceTagArgb(value) ?: 0xFF757575L)

@Composable
internal fun SentenceTagChip(
    name: String,
    color: String,
    selected: Boolean = false,
    showCheckbox: Boolean = false,
    onClick: (() -> Unit)? = null,
    modifier: Modifier = Modifier,
) {
    val tagColor = sentenceTagColor(color)
    val border = BorderStroke(
        1.dp,
        tagColor.copy(alpha = if (selected) 0.85f else 0.45f),
    )
    val content: @Composable () -> Unit = {
        Row(
            modifier = Modifier.padding(
                start = if (showCheckbox) 4.dp else 10.dp,
                end = 10.dp,
                top = if (showCheckbox) 3.dp else 6.dp,
                bottom = if (showCheckbox) 3.dp else 6.dp,
            ),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.Center,
        ) {
            if (showCheckbox) {
                Checkbox(
                    checked = selected,
                    onCheckedChange = null,
                    modifier = Modifier.size(24.dp),
                    colors = CheckboxDefaults.colors(
                        checkedColor = tagColor,
                        uncheckedColor = tagColor,
                        checkmarkColor = Color.White,
                    ),
                )
            } else {
                Box(
                    modifier = Modifier
                        .size(8.dp)
                        .clip(CircleShape),
                ) {
                    Surface(
                        color = tagColor,
                        shape = CircleShape,
                        modifier = Modifier.matchParentSize(),
                        content = {},
                    )
                }
            }
            Spacer(Modifier.width(6.dp))
            Text(
                text = name,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                fontSize = 13.sp,
            )
        }
    }
    if (onClick != null) {
        Surface(
            onClick = onClick,
            modifier = modifier,
            shape = RoundedCornerShape(8.dp),
            color = tagColor.copy(alpha = if (selected) 0.2f else 0.1f),
            contentColor = MaterialTheme.colorScheme.onSurface,
            border = border,
            content = content,
        )
    } else {
        Surface(
            modifier = modifier,
            shape = RoundedCornerShape(8.dp),
            color = tagColor.copy(alpha = 0.1f),
            contentColor = MaterialTheme.colorScheme.onSurface,
            border = border,
            content = content,
        )
    }
}

@Composable
internal fun SentenceTagBadge(
    name: String,
    color: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val tagColor = sentenceTagColor(color)
    Surface(
        onClick = onClick,
        modifier = modifier,
        shape = RoundedCornerShape(4.dp),
        color = tagColor.copy(alpha = if (selected) 0.18f else 0.07f),
        contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        border = if (selected) BorderStroke(1.dp, tagColor) else null,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 7.dp, vertical = 3.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Surface(
                modifier = Modifier.size(width = 3.dp, height = 12.dp),
                shape = RoundedCornerShape(2.dp),
                color = tagColor,
                content = {},
            )
            Spacer(Modifier.width(5.dp))
            Text(
                text = name,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                fontSize = 11.sp,
                lineHeight = 14.sp,
            )
        }
    }
}

internal fun SentenceFavoriteTag.asSentenceTag(): SentenceTag =
    SentenceTag(id = id, name = name, color = color, createdAt = "")
