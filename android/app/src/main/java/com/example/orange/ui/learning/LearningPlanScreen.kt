package com.example.orange.ui.learning

import com.example.orange.data.logging.AppLog as Log
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.example.orange.R
import com.example.orange.data.learning.LearningApi
import com.example.orange.data.learning.LearningPlan
import com.example.orange.data.learning.LearningReviewForecast
import com.example.orange.data.learning.ReviewForecastApi
import kotlinx.coroutines.launch

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun LearningPlanScreen(
    onStartLearning: (Long) -> Unit,
    onOpenSettings: () -> Unit,
    bottomPadding: Dp,
    modifier: Modifier = Modifier,
) {
    var plan by remember { mutableStateOf<LearningPlan?>(null) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var refreshAll by remember { mutableIntStateOf(0) }
    var forecast by remember { mutableStateOf<LearningReviewForecast?>(null) }
    var forecastLoading by remember { mutableStateOf(true) }
    var forecastError by remember { mutableStateOf<String?>(null) }
    var forecastRefresh by remember { mutableIntStateOf(0) }
    var extending by remember { mutableStateOf(false) }
    var extendError by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(refreshAll) {
        loading = true
        error = null
        LearningApi.createSession().fold(
            onSuccess = { plan = it },
            onFailure = {
                Log.e("LearningPlan", "create session failed", it)
                plan = null
                error = it.message ?: "加载失败"
            },
        )
        loading = false
    }
    LaunchedEffect(refreshAll, forecastRefresh) {
        forecastLoading = true
        forecastError = null
        ReviewForecastApi.get(14).fold(
            onSuccess = { forecast = it },
            onFailure = {
                Log.e("LearningPlan", "load review forecast failed", it)
                forecast = null
                forecastError = it.message ?: "加载失败"
            },
        )
        forecastLoading = false
    }

    val onExtend: () -> Unit = {
        val current = plan
        if (current != null && !extending) {
            scope.launch {
                extending = true
                extendError = null
                LearningApi.extendSession(current.sessionId).fold(
                    onSuccess = {
                        plan = it
                        forecastRefresh++
                    },
                    onFailure = {
                        Log.e("LearningPlan", "extend session failed", it)
                        extendError = it.message ?: "追加失败"
                    },
                )
                extending = false
            }
            Unit
        }
    }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text("今日学习") },
                actions = {
                    IconButton(onClick = onOpenSettings) {
                        Icon(
                            painter = painterResource(R.drawable.ic_settings),
                            contentDescription = "学习设置",
                        )
                    }
                },
            )
        },
    ) { padding ->
        Box(
            modifier = Modifier.fillMaxSize().padding(padding).padding(bottom = bottomPadding),
            contentAlignment = Alignment.Center,
        ) {
            when {
                loading -> CircularProgressIndicator()
                error != null -> Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("加载失败：$error", color = MaterialTheme.colorScheme.error)
                    TextButton(onClick = { refreshAll++ }) { Text("重试") }
                }
                plan != null -> PlanContent(
                    plan = plan!!,
                    forecast = forecast,
                    forecastLoading = forecastLoading,
                    forecastError = forecastError,
                    onRetryForecast = { forecastRefresh++ },
                    onStartLearning = onStartLearning,
                    extending = extending,
                    extendError = extendError,
                    onExtend = onExtend,
                    onDismissExtendError = { extendError = null },
                )
            }
        }
    }
}

@Composable
private fun PlanContent(
    plan: LearningPlan,
    forecast: LearningReviewForecast?,
    forecastLoading: Boolean,
    forecastError: String?,
    onRetryForecast: () -> Unit,
    onStartLearning: (Long) -> Unit,
    extending: Boolean,
    extendError: String?,
    onExtend: () -> Unit,
    onDismissExtendError: () -> Unit,
) {
    val labels = listOf("未学习", "学习中", "已学习", "已掌握")
    val values = listOf(
        plan.statusCounts.notStarted,
        plan.statusCounts.learning,
        plan.statusCounts.learned,
        plan.statusCounts.mastered,
    )
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(horizontal = 24.dp, vertical = 16.dp),
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        item {
            Surface(
                modifier = Modifier.fillMaxWidth(),
                shape = MaterialTheme.shapes.large,
                color = MaterialTheme.colorScheme.surfaceVariant,
            ) {
                Column(Modifier.padding(20.dp)) {
                    Text("今日待学习", style = MaterialTheme.typography.titleMedium)
                    Text("${plan.todayTotal}", style = MaterialTheme.typography.displaySmall)
                    Text(
                        "新词 ${plan.todayNew}  ·  复习 ${plan.todayReview}  ·  重试 ${plan.todayRetry}",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        item {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    modifier = Modifier.size(132.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    LearningStatusPieChart(
                        counts = plan.statusCounts,
                        modifier = Modifier.fillMaxSize(),
                    )
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("${plan.totalItems}", style = MaterialTheme.typography.titleLarge)
                        Text(
                            "学习总数",
                            fontSize = 10.sp,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Spacer(Modifier.width(20.dp))
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(7.dp),
                ) {
                    labels.forEachIndexed { index, label ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Box(
                                Modifier.size(7.dp).background(LearningStatusColors[index], CircleShape)
                            )
                            Text(
                                label,
                                modifier = Modifier.padding(start = 7.dp).weight(1f),
                                fontSize = 12.sp,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            Text(
                                "${values[index]}",
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold,
                            )
                        }
                    }
                }
            }
        }
        item {
            when {
                forecastLoading -> ForecastMessageCard {
                    CircularProgressIndicator(modifier = Modifier.size(28.dp))
                }
                forecastError != null -> ForecastMessageCard {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("复习预览加载失败", color = MaterialTheme.colorScheme.error)
                        TextButton(onClick = onRetryForecast) { Text("重试") }
                    }
                }
                forecast != null -> LearningReviewForecastChart(forecast)
            }
        }
        item {
            Column(
                modifier = Modifier.fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                val onClickAction: () -> Unit
                val label: String
                val enabled: Boolean
                when {
                    plan.hasCurrentCard -> {
                        onClickAction = { onStartLearning(plan.sessionId) }
                        label = "开始学习"
                        enabled = true
                    }
                    plan.canExtend -> {
                        onClickAction = onExtend
                        label = if (extending) "追加中..." else "继续学习（再学 10 个新词）"
                        enabled = !extending
                    }
                    else -> {
                        onClickAction = {}
                        label = "今日学习已完成"
                        enabled = false
                    }
                }
                Button(
                    onClick = onClickAction,
                    enabled = enabled,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (extending && plan.canExtend && !plan.hasCurrentCard) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(18.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.onPrimary,
                        )
                        Spacer(Modifier.width(8.dp))
                    }
                    Text(label)
                }
                if (extendError != null) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            "继续学习失败：$extendError",
                            color = MaterialTheme.colorScheme.error,
                            fontSize = 12.sp,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = {
                            onDismissExtendError()
                            onExtend()
                        }) { Text("重试") }
                    }
                }
            }
        }
    }
}

@Composable
private fun ForecastMessageCard(content: @Composable () -> Unit) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceVariant,
    ) {
        Box(
            modifier = Modifier.fillMaxWidth().height(176.dp),
            contentAlignment = Alignment.Center,
        ) {
            content()
        }
    }
}
