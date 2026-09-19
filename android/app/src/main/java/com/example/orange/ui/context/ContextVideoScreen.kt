package com.example.orange.ui.context

import android.annotation.SuppressLint
import android.net.Uri
import android.os.SystemClock
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import com.example.orange.R
import com.example.orange.data.contextsync.ContextVideoCacheRepository
import com.example.orange.data.learning.LearningApi
import com.example.orange.data.word.ContextVideo
import org.json.JSONObject
private fun reportContextVideoTransition(
    location: String,
    message: String,
    data: JSONObject = JSONObject(),
) {
    LearningApi.reportEvent(location, message, data)
}

@SuppressLint("UnsafeOptInUsageError")
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun ContextVideoScreen(
    video: ContextVideo,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    var error by remember(video.id) { mutableStateOf<String?>(null) }
    var cachedVideoUri by remember(video.id) { mutableStateOf<Uri?>(null) }
    var cachedSubtitleUri by remember(video.id) { mutableStateOf<Uri?>(null) }
    var playbackEnded by remember(video.id) { mutableStateOf(false) }
    val debugScreenId = remember(video.id) {
        "video-${video.id}-${SystemClock.elapsedRealtime()}"
    }
    val debugMountedAt = remember(video.id) { SystemClock.elapsedRealtime() }

    LaunchedEffect(video.id, video.videoUrl, video.subtitleUrl) {
        val debugResolveStarted = SystemClock.elapsedRealtime()
        reportContextVideoTransition(
            location = "ContextVideoScreen:resolveForPlayback",
            message = "video media resolve started",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("videoId", video.id)
                .put("elapsedRealtimeMs", debugResolveStarted),
        )
        error = null
        cachedVideoUri = null
        cachedSubtitleUri = null
        val debugResolveResult = runCatching {
            ContextVideoCacheRepository.resolveForPlayback(context, video)
        }
        reportContextVideoTransition(
            location = "ContextVideoScreen:resolveForPlayback",
            message = "video media resolve completed",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("videoId", video.id)
                .put("durationMs", SystemClock.elapsedRealtime() - debugResolveStarted)
                .put("success", debugResolveResult.isSuccess)
                .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
        )
        debugResolveResult.onSuccess { media ->
            cachedVideoUri = media.videoUri
            cachedSubtitleUri = media.subtitleUri
        }.onFailure {
            error = "视频语境加载失败"
        }
    }

    val player = remember(video.id) {
        ExoPlayer.Builder(context).build().apply {
            repeatMode = Player.REPEAT_MODE_OFF
            addListener(
                object : Player.Listener {
                    override fun onPlaybackStateChanged(playbackState: Int) {
                        playbackEnded = playbackState == Player.STATE_ENDED
                    }

                    override fun onIsPlayingChanged(isPlaying: Boolean) {
                        if (isPlaying) playbackEnded = false
                    }

                    override fun onPlayerError(playbackError: PlaybackException) {
                        error = "视频语境播放失败"
                    }
                },
            )
        }
    }

    LaunchedEffect(player, video.id, cachedVideoUri, cachedSubtitleUri) {
        val videoUri = cachedVideoUri
        if (videoUri != null) {
            playbackEnded = false
            val subtitleConfigurations = cachedSubtitleUri
                ?.let { subtitleUri ->
                listOf(
                    MediaItem.SubtitleConfiguration.Builder(
                        subtitleUri,
                    )
                        .setMimeType(MimeTypes.TEXT_VTT)
                        .setSelectionFlags(C.SELECTION_FLAG_DEFAULT)
                        .build(),
                )
                }
                .orEmpty()
            val mediaItem = MediaItem.Builder()
                .setUri(videoUri)
                .setSubtitleConfigurations(subtitleConfigurations)
                .build()
            player.setMediaItem(mediaItem)
            player.prepare()
            player.playWhenReady = true
        }
    }

    DisposableEffect(player) {
        reportContextVideoTransition(
            location = "ContextVideoScreen:lifecycle",
            message = "video screen mounted",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("videoId", video.id)
                .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
        )
        onDispose {
            val debugReleaseStarted = SystemClock.elapsedRealtime()
            reportContextVideoTransition(
                location = "ContextVideoScreen:lifecycle",
                message = "video player release started",
                data = JSONObject()
                    .put("screenId", debugScreenId)
                    .put("videoId", video.id)
                    .put("screenLifetimeMs", debugReleaseStarted - debugMountedAt)
                    .put("elapsedRealtimeMs", debugReleaseStarted),
            )
            player.stop()
            player.release()
            reportContextVideoTransition(
                location = "ContextVideoScreen:lifecycle",
                message = "video player release completed",
                data = JSONObject()
                    .put("screenId", debugScreenId)
                    .put("videoId", video.id)
                    .put("releaseDurationMs", SystemClock.elapsedRealtime() - debugReleaseStarted)
                    .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
            )
        }
    }

    val debugBack = {
        reportContextVideoTransition(
            location = "ContextVideoScreen:onBack",
            message = "video back requested",
            data = JSONObject()
                .put("screenId", debugScreenId)
                .put("videoId", video.id)
                .put("elapsedRealtimeMs", SystemClock.elapsedRealtime()),
        )
        onBack()
    }
    BackHandler(onBack = debugBack)

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text("视频语境") },
                navigationIcon = {
                    IconButton(onClick = debugBack) {
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
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .background(Color.Black),
            contentAlignment = Alignment.Center,
        ) {
            AndroidView(
                factory = { viewContext ->
                    PlayerView(viewContext).apply {
                        this.player = player
                        useController = false
                        controllerAutoShow = false
                        setShowBuffering(PlayerView.SHOW_BUFFERING_WHEN_PLAYING)
                    }
                },
                update = {
                    it.player = player
                    it.useController = false
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .aspectRatio(16f / 9f),
            )
            if (playbackEnded) {
                IconButton(
                    onClick = {
                        player.seekTo(0)
                        player.play()
                    },
                    modifier = Modifier
                        .size(64.dp)
                        .background(Color.Black.copy(alpha = 0.58f), CircleShape),
                ) {
                    Icon(
                        painter = painterResource(R.drawable.ic_play),
                        contentDescription = "重新播放",
                        tint = Color.White,
                        modifier = Modifier.size(34.dp),
                    )
                }
            }
            error?.let { message ->
                Text(
                    text = message,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .padding(16.dp),
                )
            }
            if (cachedVideoUri == null && error == null) {
                CircularProgressIndicator(color = Color.White)
            }
        }
    }
}
