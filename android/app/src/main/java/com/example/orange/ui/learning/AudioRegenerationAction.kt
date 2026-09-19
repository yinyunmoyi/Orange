package com.example.orange.ui.learning

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import com.example.orange.R

internal enum class AudioRegenerationState {
    Closed,
    Armed,
    Regenerating,
}

internal enum class AudioRegenerationEvent {
    Click,
    Succeeded,
    Failed,
    ItemChanged,
}

internal data class AudioRegenerationTransition(
    val state: AudioRegenerationState,
    val regenerate: Boolean = false,
)

internal fun reduceAudioRegenerationState(
    state: AudioRegenerationState,
    event: AudioRegenerationEvent,
): AudioRegenerationTransition {
    if (event == AudioRegenerationEvent.ItemChanged || event == AudioRegenerationEvent.Succeeded) {
        return AudioRegenerationTransition(AudioRegenerationState.Closed)
    }
    return when (state) {
        AudioRegenerationState.Closed -> when (event) {
            AudioRegenerationEvent.Click -> AudioRegenerationTransition(AudioRegenerationState.Armed)
            else -> AudioRegenerationTransition(state)
        }
        AudioRegenerationState.Armed -> when (event) {
            AudioRegenerationEvent.Click -> AudioRegenerationTransition(
                state = AudioRegenerationState.Regenerating,
                regenerate = true,
            )
            else -> AudioRegenerationTransition(state)
        }
        AudioRegenerationState.Regenerating -> when (event) {
            AudioRegenerationEvent.Failed -> AudioRegenerationTransition(AudioRegenerationState.Armed)
            else -> AudioRegenerationTransition(state)
        }
    }
}

@Composable
internal fun AudioRegenerationAction(
    state: AudioRegenerationState,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val active = state != AudioRegenerationState.Closed
    val activeColor = Color(0xFF40C4FF)
    val inactiveColor = MaterialTheme.colorScheme.onSurfaceVariant

    Box(modifier = Modifier.size(56.dp), contentAlignment = Alignment.Center) {
        if (active) {
            AudioRegenerationGlow(activeColor)
        }
        if (state == AudioRegenerationState.Regenerating) {
            CircularProgressIndicator(
                modifier = Modifier.size(25.dp),
                color = activeColor,
                strokeWidth = 2.5.dp,
            )
        } else {
            IconButton(
                onClick = onClick,
                enabled = enabled,
                modifier = Modifier.size(48.dp),
            ) {
                Icon(
                    painter = painterResource(
                        if (state == AudioRegenerationState.Armed) {
                            R.drawable.ic_dice_active
                        } else {
                            R.drawable.ic_dice
                        },
                    ),
                    contentDescription = if (state == AudioRegenerationState.Armed) {
                        "再次点击重新生成音频"
                    } else {
                        "重新生成音频"
                    },
                    modifier = Modifier
                        .size(30.dp)
                        .alpha(if (enabled) 1f else 0.55f),
                    tint = if (state == AudioRegenerationState.Closed) {
                        inactiveColor
                    } else {
                        Color.Unspecified
                    },
                )
            }
        }
    }
}

@Composable
private fun AudioRegenerationGlow(color: Color) {
    val transition = rememberInfiniteTransition(label = "audio-regeneration-glow")
    val pulse by transition.animateFloat(
        initialValue = 0.72f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = 900, easing = LinearEasing),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "audio-regeneration-glow-pulse",
    )
    Canvas(Modifier.fillMaxSize()) {
        val radius = size.minDimension * 0.5f * pulse
        drawCircle(
            brush = Brush.radialGradient(
                colors = listOf(
                    Color.White.copy(alpha = 0.48f),
                    color.copy(alpha = 0.38f),
                    color.copy(alpha = 0.12f),
                    Color.Transparent,
                ),
                center = center,
                radius = radius,
            ),
            radius = radius,
        )
    }
}
