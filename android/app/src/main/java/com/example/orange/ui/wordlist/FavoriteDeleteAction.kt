package com.example.orange.ui.wordlist

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.withTransform
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

internal enum class FavoriteDeleteState {
    Closed,
    Armed,
    Deleting,
}

internal enum class FavoriteDeleteEvent {
    Click,
    DeleteFailed,
    ItemChanged,
}

internal data class FavoriteDeleteTransition(
    val state: FavoriteDeleteState,
    val confirmDelete: Boolean = false,
)

internal fun reduceFavoriteDeleteState(
    state: FavoriteDeleteState,
    event: FavoriteDeleteEvent,
): FavoriteDeleteTransition {
    if (event == FavoriteDeleteEvent.ItemChanged) {
        return FavoriteDeleteTransition(FavoriteDeleteState.Closed)
    }
    return when (state) {
        FavoriteDeleteState.Closed -> when (event) {
            FavoriteDeleteEvent.Click -> FavoriteDeleteTransition(FavoriteDeleteState.Armed)
            else -> FavoriteDeleteTransition(state)
        }
        FavoriteDeleteState.Armed -> when (event) {
            FavoriteDeleteEvent.Click -> FavoriteDeleteTransition(
                state = FavoriteDeleteState.Deleting,
                confirmDelete = true,
            )
            else -> FavoriteDeleteTransition(state)
        }
        FavoriteDeleteState.Deleting -> when (event) {
            FavoriteDeleteEvent.DeleteFailed -> FavoriteDeleteTransition(FavoriteDeleteState.Armed)
            else -> FavoriteDeleteTransition(state)
        }
    }
}

@Composable
internal fun FavoriteDeleteAction(
    itemKey: String,
    deleteFavorite: suspend () -> Result<Unit>,
    onDeleted: () -> Unit,
    onDeleteFailed: (Throwable?) -> Unit,
) {
    var state by remember(itemKey) { mutableStateOf(FavoriteDeleteState.Closed) }
    val scope = rememberCoroutineScope()
    val lidProgress by animateFloatAsState(
        targetValue = if (state == FavoriteDeleteState.Closed) 0f else 1f,
        animationSpec = tween(durationMillis = 220),
        label = "favorite-delete-lid",
    )
    val color = LocalContentColor.current
    val description = if (state == FavoriteDeleteState.Closed) "删除" else "再次点击删除"

    IconButton(
        enabled = state != FavoriteDeleteState.Deleting,
        onClick = {
            val transition = reduceFavoriteDeleteState(state, FavoriteDeleteEvent.Click)
            state = transition.state
            if (transition.confirmDelete) {
                scope.launch {
                    deleteFavorite().fold(
                        onSuccess = { onDeleted() },
                        onFailure = { error ->
                            state = reduceFavoriteDeleteState(
                                state,
                                FavoriteDeleteEvent.DeleteFailed,
                            ).state
                            onDeleteFailed(error)
                        },
                    )
                }
            }
        },
    ) {
        Canvas(
            modifier = Modifier
                .size(24.dp)
                .semantics { contentDescription = description },
        ) {
            val unit = size.minDimension / 24f
            val stroke = 1.8f * unit
            drawRoundRect(
                color = color,
                topLeft = Offset(6f * unit, 8.5f * unit),
                size = Size(12f * unit, 12f * unit),
                cornerRadius = CornerRadius(1.8f * unit),
                style = Stroke(width = stroke),
            )
            drawLine(
                color = color,
                start = Offset(10f * unit, 11.5f * unit),
                end = Offset(10f * unit, 17.5f * unit),
                strokeWidth = stroke,
                cap = StrokeCap.Round,
            )
            drawLine(
                color = color,
                start = Offset(14f * unit, 11.5f * unit),
                end = Offset(14f * unit, 17.5f * unit),
                strokeWidth = stroke,
                cap = StrokeCap.Round,
            )
            withTransform({
                translate(left = -1.2f * unit * lidProgress, top = -2.8f * unit * lidProgress)
                rotate(
                    degrees = -32f * lidProgress,
                    pivot = Offset(6f * unit, 7f * unit),
                )
            }) {
                drawLine(
                    color = color,
                    start = Offset(5f * unit, 7f * unit),
                    end = Offset(19f * unit, 7f * unit),
                    strokeWidth = stroke,
                    cap = StrokeCap.Round,
                )
                drawLine(
                    color = color,
                    start = Offset(9f * unit, 4.5f * unit),
                    end = Offset(15f * unit, 4.5f * unit),
                    strokeWidth = stroke,
                    cap = StrokeCap.Round,
                )
                drawLine(
                    color = color,
                    start = Offset(9f * unit, 4.5f * unit),
                    end = Offset(9f * unit, 7f * unit),
                    strokeWidth = stroke,
                    cap = StrokeCap.Round,
                )
                drawLine(
                    color = color,
                    start = Offset(15f * unit, 4.5f * unit),
                    end = Offset(15f * unit, 7f * unit),
                    strokeWidth = stroke,
                    cap = StrokeCap.Round,
                )
            }
        }
    }
}
