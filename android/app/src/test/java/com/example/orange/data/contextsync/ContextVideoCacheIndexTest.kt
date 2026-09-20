package com.example.orange.data.contextsync

import java.nio.file.Files
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ContextVideoCacheIndexTest {
    @Test
    fun index_roundTripsAtomically() {
        val root = Files.createTempDirectory("context-cache-index").toFile()
        val entry = cachedEntry(videoId = 7, priority = 0)
        ContextVideoCacheIndexStore.write(
            root,
            ContextVideoCacheIndex("server", listOf(entry)),
        )

        val loaded = ContextVideoCacheIndexStore.read(root)

        assertEquals("server", loaded.serverId)
        assertEquals(listOf(entry), loaded.entries)
        assertEquals(false, root.resolve("index.json.tmp").exists())
    }

    @Test
    fun index_replacesExistingFileAtomically() {
        val root = Files.createTempDirectory("context-cache-replace").toFile()
        ContextVideoCacheIndexStore.write(
            root,
            ContextVideoCacheIndex("old-server", listOf(cachedEntry(1, priority = 2))),
        )
        val replacement = ContextVideoCacheIndex(
            "new-server",
            listOf(cachedEntry(2, priority = 0)),
        )

        ContextVideoCacheIndexStore.write(root, replacement)

        assertEquals(replacement, ContextVideoCacheIndexStore.read(root))
        assertEquals(false, root.resolve("index.json.tmp").exists())
    }

    @Test
    fun index_corruptionReturnsEmptyIndex() {
        val root = Files.createTempDirectory("context-cache-corrupt").toFile()
        root.resolve("index.json").writeText("not-json")

        val loaded = ContextVideoCacheIndexStore.read(root)

        assertTrue(loaded.entries.isEmpty())
        assertEquals(false, root.resolve("index.json").exists())
    }

    @Test
    fun evictionOrder_prefersExpiredNonDesiredEntries() {
        val now = 10_000L
        val expired = cachedEntry(1, priority = 2, retainUntil = now - 1, lastAccess = 1)
        val retained = cachedEntry(2, priority = 2, retainUntil = now + 1, lastAccess = 2)
        val desired = cachedEntry(3, priority = 0, retainUntil = now + 1, lastAccess = 3)

        val ordered = contextVideoEvictionOrder(
            listOf(desired, retained, expired),
            desiredIds = setOf(3),
            protectedVideoId = null,
            now = now,
        )

        assertEquals(listOf(1L, 2L, 3L), ordered.map { it.videoId })
    }

    @Test
    fun schedulerUsesStableUniqueWorkName() {
        assertEquals("context-video-lan-sync", ContextVideoSyncScheduler.UNIQUE_WORK)
    }

    @Test
    fun manifestCacheHit_requiresMatchingServerVersionAndSize() {
        val entry = cachedEntry(videoId = 7, priority = 0)
        val item = syncItem(videoId = 7, version = entry.version, fileSize = 100)

        assertTrue(contextVideoCacheMetadataMatches(entry, "server", item))
        assertEquals(
            false,
            contextVideoCacheMetadataMatches(entry, "other-server", item),
        )
        assertEquals(
            false,
            contextVideoCacheMetadataMatches(
                entry,
                "server",
                item.copy(version = "new-version"),
            ),
        )
        assertEquals(
            false,
            contextVideoCacheMetadataMatches(
                entry,
                "server",
                item.copy(fileSize = 101),
            ),
        )
    }

    private fun cachedEntry(
        videoId: Long,
        priority: Int,
        retainUntil: Long = 20_000,
        lastAccess: Long = 1,
    ) = CachedContextVideo(
        videoId = videoId,
        serverId = "server",
        version = "version-$videoId",
        expectedVideoBytes = 100,
        actualVideoBytes = 100,
        videoFileName = "$videoId.webm",
        subtitleFileName = "$videoId.vtt",
        createdAt = "",
        downloadedAt = 1,
        lastAccessedAt = lastAccess,
        retainUntil = retainUntil,
        priority = priority,
    )

    private fun syncItem(
        videoId: Long,
        version: String,
        fileSize: Long,
    ) = ContextVideoSyncItem(
        videoId = videoId,
        contextId = 1,
        itemType = "word",
        itemId = 1,
        durationMs = 1_000,
        fileSize = fileSize,
        version = version,
        videoUrl = "/api/v1/context-videos/$videoId/video",
        subtitleUrl = "/api/v1/context-videos/$videoId/subtitles.vtt",
        createdAt = "",
        dueAt = "",
        priority = 0,
    )
}
