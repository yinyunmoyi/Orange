package com.example.orange.ui.bookshelf

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.example.orange.data.offline.OfflineCapabilityStage
import com.example.orange.data.offline.OfflineCapabilityState

@Composable
fun OfflineCapabilityDialog(
    state: OfflineCapabilityState,
    onDismiss: () -> Unit,
    onRetry: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("离线能力验证") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                if (state.inProgress) {
                    VerificationProgress(state.stage)
                }
                state.result?.takeIf { state.stage == OfflineCapabilityStage.VERIFIED }?.let { result ->
                    Text(
                        text = "本地模型可用",
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    Text("回复：${result.reply}")
                    Text(
                        text = "后端：${result.backend}  " +
                            "初始化：${result.initializationMillis} ms  " +
                            "首字：${result.firstTokenMillis} ms  " +
                            "总计：${result.totalMillis} ms",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                state.errorMessage?.let { message ->
                    Text(
                        text = message,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        },
        confirmButton = {
            if (state.stage == OfflineCapabilityStage.ERROR) {
                TextButton(onClick = onRetry) {
                    Text("重试")
                }
            } else if (!state.inProgress) {
                TextButton(onClick = onDismiss) {
                    Text("完成")
                }
            }
        },
        dismissButton = {
            if (state.inProgress || state.stage == OfflineCapabilityStage.ERROR) {
                TextButton(onClick = onDismiss) {
                    Text(if (state.inProgress) "取消" else "关闭")
                }
            }
        },
    )
}

@Composable
private fun VerificationProgress(stage: OfflineCapabilityStage) {
    val labels = listOf(
        OfflineCapabilityStage.IMPORTING_MODEL to "导入模型",
        OfflineCapabilityStage.CHECKING_FILE to "检查文件",
        OfflineCapabilityStage.VERIFYING_MODEL to "校验模型",
        OfflineCapabilityStage.CHECKING_DEVICE to "检查设备",
        OfflineCapabilityStage.INITIALIZING_ENGINE to "初始化 GPU/CPU",
        OfflineCapabilityStage.TESTING_CONVERSATION to "测试对话",
    )
    val currentIndex = labels.indexOfFirst { it.first == stage }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        labels.forEachIndexed { index, (_, label) ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                when {
                    index < currentIndex -> Text(
                        text = "完成",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    index == currentIndex -> CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
                    )
                    else -> Text(
                        text = "等待",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(label, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}
