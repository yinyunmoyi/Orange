package com.example.orange.ui.note

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R

@Composable
fun NoteSection(
    note: String?,
    loading: Boolean,
    error: String?,
    onEdit: () -> Unit,
    modifier: Modifier = Modifier,
    titleColor: Color = MaterialTheme.colorScheme.onSurface,
    bodyColor: Color = MaterialTheme.colorScheme.onSurface,
    mutedColor: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    accentColor: Color = MaterialTheme.colorScheme.primary,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = "备注",
                color = titleColor,
                fontWeight = FontWeight.SemiBold,
                fontSize = 15.sp,
                modifier = Modifier.weight(1f),
            )
            IconButton(onClick = onEdit, enabled = !loading) {
                Icon(
                    painter = painterResource(R.drawable.ic_edit),
                    contentDescription = "编辑备注",
                    tint = accentColor,
                )
            }
        }
        when {
            loading -> CircularProgressIndicator(
                modifier = Modifier.size(20.dp),
                strokeWidth = 2.dp,
                color = accentColor,
            )
            error != null -> Text(
                text = error,
                color = mutedColor,
                fontSize = 13.sp,
                modifier = Modifier.padding(vertical = 4.dp),
            )
            note.isNullOrBlank() -> Text(
                text = "暂无备注",
                color = mutedColor,
                fontSize = 13.sp,
                modifier = Modifier.padding(vertical = 4.dp),
            )
            else -> Text(
                text = note,
                color = bodyColor,
                fontSize = 14.sp,
                lineHeight = 21.sp,
                modifier = Modifier.padding(vertical = 4.dp),
            )
        }
    }
}
