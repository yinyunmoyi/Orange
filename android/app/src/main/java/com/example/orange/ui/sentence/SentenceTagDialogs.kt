package com.example.orange.ui.sentence

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import com.example.orange.data.word.SentenceFavoriteTag
import com.example.orange.data.word.SentenceTag

internal data class SentenceTagUpdate(
    val tagIds: Set<Long>,
    val note: String,
)

internal fun buildSentenceTagUpdate(
    selectedTagIds: Set<Long>,
    note: String,
): SentenceTagUpdate? {
    if (selectedTagIds.any { it <= 0L } || selectedTagIds.size > 20) return null
    val normalizedNote = note
        .replace("\r\n", "\n")
        .replace("\r", "\n")
        .trim()
    if (normalizedNote.codePointCount(0, normalizedNote.length) > 300 ||
        normalizedNote.any { it.isISOControl() && it != '\n' } ||
        (selectedTagIds.isNotEmpty() && normalizedNote.isEmpty())
    ) {
        return null
    }
    return SentenceTagUpdate(tagIds = selectedTagIds.toSortedSet(), note = normalizedNote)
}

@Composable
internal fun CreateSentenceTagDialog(
    saving: Boolean,
    error: String?,
    onDismiss: () -> Unit,
    onConfirm: (name: String, color: String) -> Unit,
) {
    var name by remember { mutableStateOf("") }
    var color by remember { mutableStateOf("#E65100") }
    SentenceDialogFrame(
        title = "新增标签",
        saving = saving,
        error = error,
        confirmEnabled = name.trim().isNotEmpty(),
        onDismiss = onDismiss,
        onConfirm = { onConfirm(name, color) },
    ) {
        OutlinedTextField(
            value = name,
            onValueChange = { name = it.take(24) },
            modifier = Modifier.fillMaxWidth(),
            enabled = !saving,
            singleLine = true,
            label = { Text("标签名") },
            placeholder = { Text("例如：虚拟语气") },
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Sentences,
                imeAction = ImeAction.Done,
            ),
        )
        Spacer(Modifier.height(18.dp))
        Text(
            text = "标签颜色",
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            style = MaterialTheme.typography.labelLarge,
        )
        Spacer(Modifier.height(10.dp))
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            SentenceTagColorPalette.forEach { item ->
                ColorSwatch(
                    color = item,
                    selected = color == item,
                    enabled = !saving,
                    onClick = { color = item },
                )
            }
        }
    }
}

@Composable
internal fun ManageSentenceTagsDialog(
    allTags: List<SentenceTag>?,
    currentTags: List<SentenceFavoriteTag>,
    currentNote: String,
    loading: Boolean,
    saving: Boolean,
    error: String?,
    onRetry: () -> Unit,
    onDismiss: () -> Unit,
    onConfirm: (tagIds: Set<Long>, note: String) -> Unit,
) {
    val initialSelected = remember(currentTags) { currentTags.mapTo(linkedSetOf()) { it.id } }
    var selectedTagIds by remember(currentTags) { mutableStateOf<Set<Long>>(initialSelected) }
    var note by remember(currentNote) { mutableStateOf(currentNote) }
    val update = buildSentenceTagUpdate(selectedTagIds, note)

    SentenceDialogFrame(
        title = "标签与备注",
        saving = saving,
        error = error,
        confirmEnabled = !loading && allTags != null && update != null,
        onDismiss = onDismiss,
        onConfirm = { update?.let { onConfirm(it.tagIds, it.note) } },
    ) {
        OutlinedTextField(
            value = note,
            onValueChange = { note = it.take(300) },
            modifier = Modifier.fillMaxWidth(),
            enabled = !saving,
            singleLine = false,
            minLines = 3,
            maxLines = 6,
            label = { Text("句子备注") },
            placeholder = { Text("用一句话描述这句话的知识点") },
            supportingText = {
                if (selectedTagIds.isNotEmpty() && note.trim().isEmpty()) {
                    Text("选择标签时必须填写备注")
                }
            },
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Sentences,
            ),
        )
        Spacer(Modifier.height(14.dp))
        when {
            loading -> {
                CircularProgressIndicator(
                    modifier = Modifier
                        .align(Alignment.CenterHorizontally)
                        .padding(vertical = 28.dp),
                )
            }
            allTags == null -> {
                Text(
                    text = "标签加载失败",
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.align(Alignment.CenterHorizontally),
                )
                TextButton(
                    onClick = onRetry,
                    modifier = Modifier.align(Alignment.CenterHorizontally),
                ) {
                    Text("重试")
                }
            }
            allTags.isEmpty() -> {
                Text(
                    text = "暂无标签，请先在句子页创建",
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier
                        .align(Alignment.CenterHorizontally)
                        .padding(vertical = 24.dp),
                )
            }
            else -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 440.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    items(allTags, key = { it.id }) { tag ->
                        val selected = tag.id in selectedTagIds
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Checkbox(
                                checked = selected,
                                onCheckedChange = {
                                    selectedTagIds = toggleSentenceTagSelection(
                                        selectedTagIds,
                                        tag.id,
                                    )
                                },
                                enabled = !saving,
                            )
                            SentenceTagChip(
                                name = tag.name,
                                color = tag.color,
                                selected = selected,
                                onClick = {
                                    selectedTagIds = toggleSentenceTagSelection(
                                        selectedTagIds,
                                        tag.id,
                                    )
                                },
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SentenceDialogFrame(
    title: String,
    saving: Boolean,
    error: String?,
    confirmEnabled: Boolean,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    Dialog(onDismissRequest = { if (!saving) onDismiss() }) {
        Surface(
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(8.dp),
            tonalElevation = 6.dp,
        ) {
            Column(
                modifier = Modifier.padding(
                    top = 24.dp,
                    start = 20.dp,
                    end = 20.dp,
                    bottom = 12.dp,
                ),
            ) {
                Text(
                    text = title,
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.align(Alignment.CenterHorizontally),
                )
                Spacer(Modifier.height(20.dp))
                content()
                if (error != null) {
                    Text(
                        text = error,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = 10.dp),
                    )
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 12.dp),
                    horizontalArrangement = Arrangement.End,
                ) {
                    TextButton(onClick = onDismiss, enabled = !saving) {
                        Text("取消")
                    }
                    Spacer(Modifier.width(12.dp))
                    TextButton(
                        onClick = onConfirm,
                        enabled = !saving && confirmEnabled,
                    ) {
                        if (saving) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(18.dp),
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

@Composable
private fun ColorSwatch(
    color: String,
    selected: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val swatchColor = sentenceTagColor(color)
    Surface(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier.size(34.dp),
        shape = CircleShape,
        color = swatchColor,
        border = BorderStroke(
            width = if (selected) 3.dp else 1.dp,
            color = if (selected) MaterialTheme.colorScheme.onSurface else Color.Transparent,
        ),
        content = {},
    )
}
