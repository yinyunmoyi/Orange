package com.example.orange.data.contextsync

import android.content.Context
import android.net.Uri
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.WordApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File
import java.net.URL

data class CachedContextMedia(
    val videoUri: Uri,
    val subtitleUri: Uri?,
)

data class ContextVideoSyncSummary(
    val hits: Int,
    val migrated: Int,
    val downloaded: Int,
    val failed: Int,
    val evicted: Int,
    val downloadedBytes: Long,
)

object ContextVideoCacheRepository {
    private val mutex = Mutex()

    suspend fun sync(
        context: Context,
        endpoint: ContextSyncEndpoint,
        manifest: ContextVideoSyncManifest,
    ): ContextVideoSyncSummary = withContext(Dispatchers.IO) {
        mutex.withLock {
            val root = root(context)
            prepareDirectories(root)
            var index = ContextVideoCacheIndexStore.read(root)
            if (
                index.serverId.isNotEmpty() &&
                index.serverId != endpoint.serverId
            ) {
                clearCache(root)
                prepareDirectories(root)
                index = ContextVideoCacheIndex(endpoint.serverId, emptyList())
            }
            val entries = index.entries.associateBy { it.videoId }.toMutableMap()
            cleanupOrphans(root, entries.values)
            var hits = 0
            var migrated = 0
            var downloaded = 0
            var failed = 0
            var evicted = 0
            var downloadedBytes = 0L
            val now = System.currentTimeMillis()
            val retainUntil = now + RETAIN_MILLIS
            val desiredIds = manifest.items.mapTo(mutableSetOf()) { it.videoId }

            for (item in manifest.items) {
                val existing = entries[item.videoId]
                if (
                    existing != null &&
                    contextVideoCacheMetadataMatches(existing, endpoint.serverId, item) &&
                    cachedFilesValid(root, existing, item.fileSize)
                ) {
                    entries[item.videoId] = existing.copy(
                        expectedVideoBytes = item.fileSize,
                        retainUntil = retainUntil,
                        priority = item.priority,
                    )
                    hits++
                    continue
                }
                if (existing != null) {
                    deleteEntryFiles(root, existing)
                    entries.remove(item.videoId)
                }
                if (promoteLegacy(context, root, item, endpoint.serverId, retainUntil)?.also {
                        entries[item.videoId] = it
                    } != null
                ) {
                    migrated++
                    continue
                }

                val required = item.fileSize + MIN_FREE_BYTES
                evicted += trimToBudget(
                    root = root,
                    entries = entries,
                    desiredIds = desiredIds,
                    protectedVideoId = item.videoId,
                    targetBytes = CACHE_BUDGET_BYTES - item.fileSize,
                    now = now,
                )
                if (root.usableSpace < required) break
                runCatching {
                    downloadManifestItem(root, endpoint, item, retainUntil)
                }.onSuccess { entry ->
                    existing?.let { deleteEntryFiles(root, it) }
                    entries[item.videoId] = entry
                    downloaded++
                    downloadedBytes += entry.actualVideoBytes
                }.onFailure {
                    failed++
                }
            }
            evicted += trimToBudget(
                root,
                entries,
                desiredIds,
                protectedVideoId = null,
                targetBytes = CACHE_BUDGET_BYTES,
                now = now,
            )
            ContextVideoCacheIndexStore.write(
                root,
                ContextVideoCacheIndex(endpoint.serverId, entries.values.toList()),
            )
            ContextVideoSyncSummary(
                hits = hits,
                migrated = migrated,
                downloaded = downloaded,
                failed = failed,
                evicted = evicted,
                downloadedBytes = downloadedBytes,
            )
        }
    }

    suspend fun resolveForPlayback(
        context: Context,
        video: ContextVideo,
    ): CachedContextMedia = withContext(Dispatchers.IO) {
        mutex.withLock {
            val root = root(context)
            prepareDirectories(root)
            val preferences = ContextSyncPreferences(context)
            var index = ContextVideoCacheIndexStore.read(root)
            val preferredServerId = preferences.serverId
            if (
                preferredServerId != null &&
                index.serverId.isNotEmpty() &&
                index.serverId != UNBOUND_SERVER &&
                index.serverId != preferredServerId
            ) {
                clearCache(root)
                prepareDirectories(root)
                index = ContextVideoCacheIndex(preferredServerId, emptyList())
            }
            val entries = index.entries.associateBy { it.videoId }.toMutableMap()
            entries[video.id]?.takeIf { cachedFilesValid(root, it, null) }?.let { hit ->
                val updated = hit.copy(lastAccessedAt = System.currentTimeMillis())
                entries[video.id] = updated
                ContextVideoCacheIndexStore.write(root, index.copy(entries = entries.values.toList()))
                return@withLock media(root, updated)
            }

            promoteLegacyWithoutManifest(context, root, video, index.serverId)?.let { migrated ->
                entries[video.id] = migrated
                ContextVideoCacheIndexStore.write(root, index.copy(entries = entries.values.toList()))
                return@withLock media(root, migrated)
            }

            val localEndpoint = preferences.localOrigin?.let { origin ->
                preferences.serverId?.let { serverId ->
                    ContextVideoSyncApi.ping(origin).getOrNull()
                        ?.takeIf { it == serverId }
                        ?.let { ContextSyncEndpoint(origin, serverId) }
                }
            }
            if (
                localEndpoint != null &&
                index.serverId.isNotEmpty() &&
                index.serverId != localEndpoint.serverId
            ) {
                clearCache(root)
                prepareDirectories(root)
                entries.clear()
                index = ContextVideoCacheIndex(localEndpoint.serverId, emptyList())
            }
            val downloaded = if (localEndpoint != null) {
                runCatching {
                    downloadOnDemand(
                        root,
                        localEndpoint.origin,
                        video,
                        localEndpoint.serverId,
                    )
                }.getOrElse {
                    downloadOnDemand(
                        root,
                        absoluteOrigin(video.videoUrl),
                        video,
                        localEndpoint.serverId,
                    )
                }
            } else {
                downloadOnDemand(
                    root,
                    absoluteOrigin(video.videoUrl),
                    video,
                    UNBOUND_SERVER,
                )
            }
            entries[video.id]?.let { deleteEntryFiles(root, it) }
            entries[video.id] = downloaded
            trimToBudget(
                root,
                entries,
                emptySet(),
                protectedVideoId = video.id,
                targetBytes = CACHE_BUDGET_BYTES,
                now = System.currentTimeMillis(),
            )
            ContextVideoCacheIndexStore.write(
                root,
                ContextVideoCacheIndex(
                    localEndpoint?.serverId ?: index.serverId.ifEmpty { UNBOUND_SERVER },
                    entries.values.toList(),
                ),
            )
            media(root, downloaded)
        }
    }

    private fun downloadManifestItem(
        root: File,
        endpoint: ContextSyncEndpoint,
        item: ContextVideoSyncItem,
        retainUntil: Long,
    ): CachedContextVideo {
        val suffix = item.version.take(12)
        val videoName = "${item.videoId}-$suffix.webm"
        val subtitleName = "${item.videoId}-$suffix.vtt"
        val videoTemp = File(root, "tmp/$videoName.part")
        val subtitleTemp = File(root, "tmp/$subtitleName.part")
        try {
            val size = ContextVideoDownloader.downloadVideo(
                endpoint.origin,
                item.videoUrl,
                videoTemp,
                item.fileSize,
            )
            ContextVideoDownloader.downloadVtt(
                endpoint.origin,
                item.subtitleUrl,
                subtitleTemp,
            )
            val videoTarget = File(root, "media/$videoName")
            val subtitleTarget = File(root, "subtitles/$subtitleName")
            commit(videoTemp, videoTarget)
            commit(subtitleTemp, subtitleTarget)
            val now = System.currentTimeMillis()
            return CachedContextVideo(
                videoId = item.videoId,
                serverId = endpoint.serverId,
                version = item.version,
                expectedVideoBytes = item.fileSize,
                actualVideoBytes = size,
                videoFileName = videoName,
                subtitleFileName = subtitleName,
                createdAt = item.createdAt,
                downloadedAt = now,
                lastAccessedAt = now,
                retainUntil = retainUntil,
                priority = item.priority,
            )
        } finally {
            videoTemp.delete()
            subtitleTemp.delete()
        }
    }

    private fun downloadOnDemand(
        root: File,
        origin: String,
        video: ContextVideo,
        serverId: String,
    ): CachedContextVideo {
        val version = "ondemand-${video.id}"
        val videoName = "${video.id}-ondemand.webm"
        val subtitleName = "${video.id}-ondemand.vtt"
        val videoTemp = File(root, "tmp/$videoName.part")
        val subtitleTemp = File(root, "tmp/$subtitleName.part")
        val videoPath = pathFromAbsoluteOrRelative(video.videoUrl)
        val subtitlePath = pathFromAbsoluteOrRelative(video.subtitleUrl)
        try {
            val size = ContextVideoDownloader.downloadVideo(
                origin,
                videoPath,
                videoTemp,
                null,
            )
            if (subtitlePath.isNotEmpty()) {
                ContextVideoDownloader.downloadVtt(origin, subtitlePath, subtitleTemp)
            }
            commit(videoTemp, File(root, "media/$videoName"))
            if (subtitlePath.isNotEmpty()) {
                commit(subtitleTemp, File(root, "subtitles/$subtitleName"))
            } else {
                File(root, "subtitles/$subtitleName").writeText("WEBVTT\n\n")
            }
            val now = System.currentTimeMillis()
            return CachedContextVideo(
                videoId = video.id,
                serverId = serverId,
                version = version,
                expectedVideoBytes = size,
                actualVideoBytes = size,
                videoFileName = videoName,
                subtitleFileName = subtitleName,
                createdAt = "",
                downloadedAt = now,
                lastAccessedAt = now,
                retainUntil = now + RETAIN_MILLIS,
                priority = 2,
            )
        } finally {
            videoTemp.delete()
            subtitleTemp.delete()
        }
    }

    private fun promoteLegacy(
        context: Context,
        root: File,
        item: ContextVideoSyncItem,
        serverId: String,
        retainUntil: Long,
    ): CachedContextVideo? {
        val legacyVideo = File(context.cacheDir, "context-videos/${item.videoId}.webm")
        val legacyVtt = File(context.cacheDir, "context-videos/${item.videoId}.vtt")
        if (legacyVideo.length() != item.fileSize || !validVtt(legacyVtt)) return null
        val suffix = item.version.take(12)
        val videoName = "${item.videoId}-$suffix.webm"
        val subtitleName = "${item.videoId}-$suffix.vtt"
        moveOrCopy(legacyVideo, File(root, "media/$videoName"))
        moveOrCopy(legacyVtt, File(root, "subtitles/$subtitleName"))
        val now = System.currentTimeMillis()
        return CachedContextVideo(
            item.videoId, serverId, item.version, item.fileSize, item.fileSize,
            videoName, subtitleName, item.createdAt, now, now, retainUntil, item.priority,
        )
    }

    private fun promoteLegacyWithoutManifest(
        context: Context,
        root: File,
        video: ContextVideo,
        serverId: String,
    ): CachedContextVideo? {
        val legacyVideo = File(context.cacheDir, "context-videos/${video.id}.webm")
        val legacyVtt = File(context.cacheDir, "context-videos/${video.id}.vtt")
        if (!legacyVideo.isFile || legacyVideo.length() <= 0L || !validVtt(legacyVtt)) return null
        val legacyVideoSize = legacyVideo.length()
        val videoName = "${video.id}-legacy.webm"
        val subtitleName = "${video.id}-legacy.vtt"
        moveOrCopy(legacyVideo, File(root, "media/$videoName"))
        moveOrCopy(legacyVtt, File(root, "subtitles/$subtitleName"))
        val now = System.currentTimeMillis()
        return CachedContextVideo(
            video.id, serverId.ifEmpty { UNBOUND_SERVER }, "legacy-${video.id}",
            legacyVideoSize, File(root, "media/$videoName").length(),
            videoName, subtitleName, "", now, now, now + RETAIN_MILLIS, 2,
        )
    }

    private fun trimToBudget(
        root: File,
        entries: MutableMap<Long, CachedContextVideo>,
        desiredIds: Set<Long>,
        protectedVideoId: Long?,
        targetBytes: Long,
        now: Long,
    ): Int {
        var total = entries.values.sumOf { entryBytes(root, it) }
        if (total <= targetBytes) return 0
        val candidates = contextVideoEvictionOrder(
            entries.values,
            desiredIds,
            protectedVideoId,
            now,
        )
        var evicted = 0
        for (entry in candidates) {
            if (total <= targetBytes) break
            total -= entryBytes(root, entry)
            deleteEntryFiles(root, entry)
            entries.remove(entry.videoId)
            evicted++
        }
        return evicted
    }

    private fun cachedFilesValid(
        root: File,
        entry: CachedContextVideo,
        expectedBytes: Long?,
    ): Boolean {
        val video = File(root, "media/${entry.videoFileName}")
        val subtitle = File(root, "subtitles/${entry.subtitleFileName}")
        return video.isFile &&
            video.length() > 0L &&
            (expectedBytes == null || video.length() == expectedBytes) &&
            validVtt(subtitle)
    }

    private fun validVtt(file: File): Boolean =
        file.isFile &&
            file.length() >= 6L &&
            runCatching {
                file.inputStream().buffered().use { input ->
                    val prefix = ByteArray(6)
                    input.read(prefix) == 6 && prefix.toString(Charsets.UTF_8) == "WEBVTT"
                }
            }.getOrDefault(false)

    private fun cleanupOrphans(root: File, entries: Collection<CachedContextVideo>) {
        File(root, "tmp").listFiles()?.forEach(File::delete)
        val keepVideo = entries.mapTo(mutableSetOf()) { it.videoFileName }
        val keepSubtitle = entries.mapTo(mutableSetOf()) { it.subtitleFileName }
        File(root, "media").listFiles()?.filter { it.name !in keepVideo }?.forEach(File::delete)
        File(root, "subtitles").listFiles()?.filter { it.name !in keepSubtitle }?.forEach(File::delete)
    }

    private fun clearCache(root: File) {
        root.deleteRecursively()
    }

    private fun prepareDirectories(root: File) {
        root.mkdirs()
        File(root, "media").mkdirs()
        File(root, "subtitles").mkdirs()
        File(root, "tmp").mkdirs()
    }

    private fun commit(source: File, target: File) {
        target.parentFile?.mkdirs()
        target.delete()
        check(source.renameTo(target)) { "unable to commit cached media" }
    }

    private fun moveOrCopy(source: File, target: File) {
        target.parentFile?.mkdirs()
        target.delete()
        if (!source.renameTo(target)) {
            source.copyTo(target, overwrite = true)
            source.delete()
        }
    }

    private fun entryBytes(root: File, entry: CachedContextVideo): Long =
        File(root, "media/${entry.videoFileName}").length() +
            File(root, "subtitles/${entry.subtitleFileName}").length()

    private fun deleteEntryFiles(root: File, entry: CachedContextVideo) {
        File(root, "media/${entry.videoFileName}").delete()
        File(root, "subtitles/${entry.subtitleFileName}").delete()
    }

    private fun media(root: File, entry: CachedContextVideo): CachedContextMedia =
        CachedContextMedia(
            videoUri = Uri.fromFile(File(root, "media/${entry.videoFileName}")),
            subtitleUri = File(root, "subtitles/${entry.subtitleFileName}")
                .takeIf(File::isFile)
                ?.let(Uri::fromFile),
        )

    private fun root(context: Context): File =
        File(context.filesDir, "context-video-cache")

    private fun absoluteOrigin(rawUrl: String): String {
        val absolute = WordApi.mediaAbsoluteUrl(rawUrl)
        val url = URL(absolute)
        return "${url.protocol}://${url.authority}"
    }

    private fun pathFromAbsoluteOrRelative(rawUrl: String): String {
        if (rawUrl.isSafeApiPath()) return rawUrl
        val url = URL(WordApi.mediaAbsoluteUrl(rawUrl))
        return buildString {
            append(url.path)
            if (url.query != null) append('?').append(url.query)
        }
    }

    private const val CACHE_BUDGET_BYTES = 10_737_418_240L
    private const val MIN_FREE_BYTES = 64L * 1024L * 1024L
    private const val RETAIN_MILLIS = 7L * 24L * 60L * 60L * 1000L
    private const val UNBOUND_SERVER = "unbound"
}

internal fun contextVideoEvictionOrder(
    entries: Collection<CachedContextVideo>,
    desiredIds: Set<Long>,
    protectedVideoId: Long?,
    now: Long,
): List<CachedContextVideo> =
    entries
        .filter { it.videoId != protectedVideoId }
        .sortedWith(
            compareBy<CachedContextVideo>(
                { it.videoId in desiredIds },
                { it.retainUntil > now },
                { -it.priority },
                { it.lastAccessedAt },
            ),
        )

internal fun contextVideoCacheMetadataMatches(
    entry: CachedContextVideo,
    serverId: String,
    item: ContextVideoSyncItem,
): Boolean =
    entry.videoId == item.videoId &&
        entry.serverId == serverId &&
        entry.version == item.version &&
        entry.expectedVideoBytes == item.fileSize
