package com.example.orange.data.audiosync

import android.content.Context
import com.example.orange.data.contextsync.ContextSyncEndpoint
import com.example.orange.data.word.WordApi
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File
import java.net.URI
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicLong

data class AudioSyncSummary(
    val hits: Int,
    val downloaded: Int,
    val failed: Int,
    val deleted: Int,
    val downloadedBytes: Long,
)

object AudioCacheRepository {
    private val stateMutex = Mutex()
    private val syncMutex = Mutex()
    private val epoch = AtomicLong(0L)
    private var loadedRoot: String? = null
    private var state = AudioCacheIndex("", emptyList())

    suspend fun sync(
        context: Context,
        endpoint: ContextSyncEndpoint,
        manifest: AudioSyncManifest,
    ): AudioSyncSummary = syncMutex.withLock {
        val root = root(context)
        mediaDir(root)
        val snapshot = stateMutex.withLock {
            load(root)
            if (state.serverId.isNotEmpty() && state.serverId != manifest.serverId) {
                clearFiles(root)
                state = AudioCacheIndex(manifest.serverId, emptyList())
                AudioCacheIndexStore.write(root, state)
                epoch.incrementAndGet()
            } else if (state.serverId.isEmpty()) {
                state = state.copy(serverId = manifest.serverId)
            }
            state
        }
        val startEpoch = epoch.get()
        val existing = snapshot.entries.associateBy(::cacheKey)
        var hits = 0
        val missing = manifest.items.filter { item ->
            val cached = existing[cacheKey(item)]
            val hit = cached != null &&
                cached.version == item.version &&
                cached.playbackUrl == normalizePlaybackUrl(item.playbackUrl) &&
                cachedFile(root, cached).let { it.isFile && it.length() == cached.actualBytes }
            if (hit) hits++
            !hit
        }

        val downloaded = mutableMapOf<String, CachedAudio>()
        var failed = 0
        var downloadedBytes = 0L
        for (batch in missing.chunked(MAX_PARALLEL_DOWNLOADS)) {
            val results = coroutineScope {
                batch.map { item ->
                    async {
                        runCatching { downloadManifestItem(root, endpoint, item) }
                    }
                }.map { it.await() }
            }
            results.forEachIndexed { index, result ->
                result.fold(
                    onSuccess = { entry ->
                        downloaded[cacheKey(batch[index])] = entry
                        downloadedBytes += entry.actualBytes
                    },
                    onFailure = { failed++ },
                )
            }
        }

        if (epoch.get() != startEpoch) {
            downloaded.values.forEach { cachedFile(root, it).delete() }
            return@withLock AudioSyncSummary(hits, 0, failed + downloaded.size, 0, 0L)
        }

        stateMutex.withLock {
            load(root)
            val current = state.entries.associateBy(::cacheKey).toMutableMap()
            val manifestKeys = manifest.items.mapTo(mutableSetOf(), ::cacheKey)
            var deleted = 0

            for (item in manifest.items) {
                val key = cacheKey(item)
                val replacement = downloaded[key] ?: continue
                current.put(key, replacement)?.let { old ->
                    if (old.fileName != replacement.fileName && cachedFile(root, old).delete()) {
                        deleted++
                    }
                }
            }

            val manifestItemKeys = manifest.items
                .mapTo(mutableSetOf()) { "${it.itemType}:${it.itemId}" }
            val replacedItemKeys = downloaded.values
                .mapTo(mutableSetOf()) { "${it.itemType}:${it.itemId}" }
            val iterator = current.iterator()
            while (iterator.hasNext()) {
                val (key, entry) = iterator.next()
                val itemKey = "${entry.itemType}:${entry.itemId}"
                val remove = when {
                    entry.audioId < 0L ->
                        itemKey !in manifestItemKeys || itemKey in replacedItemKeys
                    else -> key !in manifestKeys
                }
                if (remove) {
                    if (cachedFile(root, entry).delete()) deleted++
                    iterator.remove()
                }
            }

            val entries = current.values
                .filter { cachedFile(root, it).let { file -> file.isFile && file.length() == it.actualBytes } }
                .sortedWith(compareBy<CachedAudio> { it.itemType }.thenBy { it.itemId }.thenBy { it.audioId })
            state = AudioCacheIndex(manifest.serverId, entries)
            AudioCacheIndexStore.write(root, state)
            deleted += deleteUnknownFiles(root, entries)
            return@withLock AudioSyncSummary(
                hits = hits,
                downloaded = downloaded.size,
                failed = failed,
                deleted = deleted,
                downloadedBytes = downloadedBytes,
            )
        }
    }

    suspend fun resolve(context: Context, rawUrl: String): String? {
        val normalized = normalizePlaybackUrl(rawUrl)
        if (normalized == null) return null
        val root = root(context)
        return stateMutex.withLock {
            load(root)
            val entry = state.entries.lastOrNull { it.playbackUrl == normalized } ?: return@withLock null
            val file = cachedFile(root, entry)
            if (file.isFile && file.length() == entry.actualBytes) return@withLock file.absolutePath
            state = state.copy(entries = state.entries - entry)
            AudioCacheIndexStore.write(root, state)
            null
        }
    }

    suspend fun invalidate(context: Context, itemType: String, itemId: Long) {
        if (itemType !in setOf("word", "phrase") || itemId <= 0L) return
        val root = root(context)
        stateMutex.withLock {
            load(root)
            val removed = state.entries.filter { it.itemType == itemType && it.itemId == itemId }
            epoch.incrementAndGet()
            if (removed.isEmpty()) return@withLock
            removed.forEach { cachedFile(root, it).delete() }
            state = state.copy(entries = withoutAudioItem(state.entries, itemType, itemId))
            AudioCacheIndexStore.write(root, state)
        }
    }

    suspend fun cacheRegenerated(
        context: Context,
        itemType: String,
        itemId: Long,
        rawUrl: String,
    ): String? {
        val playbackUrl = normalizePlaybackUrl(rawUrl) ?: return null
        val root = root(context)
        mediaDir(root)
        if (root.usableSpace < MIN_FREE_BYTES) return null
        val version = sha256(rawUrl)
        val fileName = "$itemType-${-itemId}-$version.mp3"
        val target = File(mediaDir(root), fileName)
        val temporary = File(mediaDir(root), "$fileName.tmp")
        return runCatching {
            temporary.delete()
            val bytes = AudioSyncApi.download(WordApi.mediaAbsoluteUrl(rawUrl), temporary)
            check(temporary.renameTo(target)) { "unable to commit regenerated audio" }
            val entry = CachedAudio(
                itemType = itemType,
                itemId = itemId,
                audioId = -itemId,
                accent = "",
                version = version,
                playbackUrl = playbackUrl,
                fileName = fileName,
                actualBytes = bytes,
            )
            stateMutex.withLock {
                load(root)
                val retained = state.entries.filterNot {
                    it.itemType == itemType && it.itemId == itemId && it.audioId < 0L
                }
                state = state.copy(entries = retained + entry)
                AudioCacheIndexStore.write(root, state)
            }
            target.absolutePath
        }.getOrElse {
            temporary.delete()
            null
        }
    }

    private suspend fun downloadManifestItem(
        root: File,
        endpoint: ContextSyncEndpoint,
        item: AudioSyncItem,
    ): CachedAudio {
        check(root.usableSpace >= item.fileSize + MIN_FREE_BYTES) { "insufficient storage" }
        val cacheAudioId = if (item.itemType == "word") item.audioId else item.itemId
        val fileName = "${item.itemType}-$cacheAudioId-${item.version}.mp3"
        val target = File(mediaDir(root), fileName)
        if (target.isFile && target.length() == item.fileSize) {
            return item.toCached(fileName, item.fileSize)
        }
        val temporary = File(mediaDir(root), "$fileName.${System.nanoTime()}.tmp")
        temporary.delete()
        return try {
            val bytes = AudioSyncApi.download(
                endpoint.origin + item.audioUrl,
                temporary,
                item.fileSize,
            )
            if (target.exists()) target.delete()
            check(temporary.renameTo(target)) { "unable to commit cached audio" }
            item.toCached(fileName, bytes)
        } finally {
            temporary.delete()
        }
    }

    private fun AudioSyncItem.toCached(fileName: String, bytes: Long): CachedAudio =
        CachedAudio(
            itemType = itemType,
            itemId = itemId,
            audioId = if (itemType == "word") audioId else itemId,
            accent = accent,
            version = version,
            playbackUrl = normalizePlaybackUrl(playbackUrl) ?: error("invalid playback URL"),
            fileName = fileName,
            actualBytes = bytes,
        )

    private fun load(root: File) {
        if (loadedRoot == root.absolutePath) return
        state = AudioCacheIndexStore.read(root)
        loadedRoot = root.absolutePath
    }

    private fun cacheKey(item: AudioSyncItem): String =
        "${item.itemType}:${if (item.itemType == "word") item.audioId else item.itemId}"

    private fun cacheKey(entry: CachedAudio): String = "${entry.itemType}:${entry.audioId}"

    private fun cachedFile(root: File, entry: CachedAudio): File =
        File(mediaDir(root), entry.fileName)

    private fun root(context: Context): File =
        File(context.applicationContext.filesDir, "word-audio-cache")

    private fun mediaDir(root: File): File =
        File(root, "media").also(File::mkdirs)

    private fun clearFiles(root: File) {
        root.listFiles()?.forEach { it.deleteRecursively() }
        mediaDir(root)
    }

    private fun deleteUnknownFiles(root: File, entries: List<CachedAudio>): Int {
        val keep = entries.mapTo(mutableSetOf()) { it.fileName }
        var deleted = 0
        mediaDir(root).listFiles()?.forEach { file ->
            if (file.name !in keep && file.delete()) deleted++
        }
        return deleted
    }

    internal fun normalizePlaybackUrl(rawUrl: String): String? = runCatching {
        val uri = URI(rawUrl)
        val path = uri.rawPath?.takeIf { it.isSafeAudioApiPath() } ?: return null
        val query = uri.rawQuery
            ?.split('&')
            ?.filter { it.isNotBlank() && it.substringBefore('=') != "v" }
            ?.joinToString("&")
            .orEmpty()
        if (query.isEmpty()) path else "$path?$query"
    }.getOrNull()

    private fun sha256(value: String): String =
        MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }

    private const val MAX_PARALLEL_DOWNLOADS = 4
    private const val MIN_FREE_BYTES = 16L * 1024L * 1024L
}

internal fun withoutAudioItem(
    entries: List<CachedAudio>,
    itemType: String,
    itemId: Long,
): List<CachedAudio> = entries.filterNot { it.itemType == itemType && it.itemId == itemId }
