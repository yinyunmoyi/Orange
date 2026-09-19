package com.example.orange.ui.learning

import com.example.orange.data.logging.AppLog as Log
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.learning.LearningApi
import com.example.orange.data.learning.LearningSettings
import kotlinx.coroutines.launch

private enum class LearningSettingDialog {
    DailyNew,
    DailyReview,
}

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun LearningSettingsScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var settings by remember { mutableStateOf<LearningSettings?>(null) }
    var loading by remember { mutableStateOf(true) }
    var loadError by remember { mutableStateOf<String?>(null) }
    var loadTick by remember { mutableIntStateOf(0) }
    var dialog by remember { mutableStateOf<LearningSettingDialog?>(null) }
    var saving by remember { mutableStateOf(false) }
    var saveError by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(loadTick) {
        loading = true
        loadError = null
        LearningApi.getSettings().fold(
            onSuccess = { settings = it },
            onFailure = {
                Log.e(TAG, "load settings failed", it)
                loadError = it.message ?: "加载失败"
            },
        )
        loading = false
    }

    fun saveSettings(dailyNewLimit: Int, dailyReviewLimit: Int) {
        if (saving) return
        saving = true
        saveError = null
        scope.launch {
            LearningApi.updateSettings(dailyNewLimit, dailyReviewLimit).fold(
                onSuccess = {
                    settings = it
                    dialog = null
                },
                onFailure = {
                    Log.e(
                        TAG,
                        "save settings failed dailyNew=$dailyNewLimit dailyReview=$dailyReviewLimit",
                        it,
                    )
                    saveError = it.message ?: "保存失败，请重试"
                },
            )
            saving = false
        }
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("学习设置") },
                navigationIcon = {
                    IconButton(onClick = onBack, enabled = !saving) {
                        Icon(
                            painter = painterResource(R.drawable.ic_back),
                            contentDescription = "返回",
                        )
                    }
                },
            )
        },
    ) { padding ->
        Box(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentAlignment = Alignment.Center,
        ) {
            when {
                loading -> CircularProgressIndicator()
                loadError != null -> Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(
                        "加载失败：$loadError",
                        color = MaterialTheme.colorScheme.error,
                    )
                    TextButton(onClick = { loadTick++ }) { Text("重试") }
                }
                settings != null -> Column(
                    modifier = Modifier.fillMaxSize().padding(top = 8.dp),
                ) {
                    LearningSettingRow(
                        title = "每天学习新词数",
                        subtitle = "每天学习 ${settings!!.dailyNewLimit} 个新词",
                        onClick = {
                            saveError = null
                            dialog = LearningSettingDialog.DailyNew
                        },
                    )
                    HorizontalDivider(modifier = Modifier.padding(horizontal = 20.dp))
                    LearningSettingRow(
                        title = "每天复习上限",
                        subtitle = "最多复习 ${settings!!.dailyReviewLimit} 个单词",
                        onClick = {
                            saveError = null
                            dialog = LearningSettingDialog.DailyReview
                        },
                    )
                }
            }
        }
    }

    val current = settings
    if (current != null) {
        when (dialog) {
            LearningSettingDialog.DailyNew -> DailyNewLimitDialog(
                currentValue = current.dailyNewLimit,
                remainingNewCount = current.remainingNewCount,
                studyDate = current.studyDate,
                saving = saving,
                error = saveError,
                onDismiss = {
                    dialog = null
                    saveError = null
                },
                onConfirm = { saveSettings(it, current.dailyReviewLimit) },
            )
            LearningSettingDialog.DailyReview -> DailyReviewLimitDialog(
                currentValue = current.dailyReviewLimit,
                saving = saving,
                error = saveError,
                onDismiss = {
                    dialog = null
                    saveError = null
                },
                onConfirm = { saveSettings(current.dailyNewLimit, it) },
            )
            null -> Unit
        }
    }
}

@Composable
private fun LearningSettingRow(
    title: String,
    subtitle: String,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 24.dp, vertical = 18.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            Text(
                subtitle,
                modifier = Modifier.padding(top = 4.dp),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                fontSize = 14.sp,
            )
        }
        Icon(
            painter = painterResource(R.drawable.ic_chevron_right),
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private const val TAG = "LearningSettings"
