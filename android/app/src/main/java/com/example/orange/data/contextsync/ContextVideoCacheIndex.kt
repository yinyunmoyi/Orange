package com.example.orange.data.contextsync

import org.json.JSONArray
import org.json.JSONObject
import java.io.File

internal data class CachedContextVideo(
    val videoId: Long,
    val serverId: String,
    val version: String,
    val expectedVideoBytes: Long,
    val actualVideoBytes: Long,
    val videoFileName: String,
    val subtitleFileName: String,
    val createdAt: String,
    val downloadedAt: Long,
    val lastAccessedAt: Long,
    val retainUntil: Long,
    val priority: Int,
)

internal data class ContextVideoCacheIndex(
    val serverId: String,
    val entries: List<CachedContextVideo>,
)

internal object ContextVideoCacheIndexStore {
    fun read(root: File): ContextVideoCacheIndex {
        val file = File(root, INDEX_FILE)
        if (!file.isFile) return ContextVideoCacheIndex("", emptyList())
        return runCatching {
            val json = JSONObject(file.readText())
            require(json.optInt("schemaVersion") == SCHEMA_VERSION)
            val serverId = json.optString("serverId")
            val entries = buildList {
                val array = json.optJSONArray("entries") ?: JSONArray()
                for (index in 0 until array.length()) {
                    val item = array.optJSONObject(index) ?: continue
                    val entry = CachedContextVideo(
                        videoId = item.optLong("videoId"),
                        serverId = item.optString("serverId"),
                        version = item.optString("version"),
                        expectedVideoBytes = item.optLong("expectedVideoBytes"),
                        actualVideoBytes = item.optLong("actualVideoBytes"),
                        videoFileName = item.optString("videoFileName"),
                        subtitleFileName = item.optString("subtitleFileName"),
                        createdAt = item.optString("createdAt"),
                        downloadedAt = item.optLong("downloadedAt"),
                        lastAccessedAt = item.optLong("lastAccessedAt"),
                        retainUntil = item.optLong("retainUntil"),
                        priority = item.optInt("priority", 2).coerceIn(0, 2),
                    )
                    if (entry.isValid()) add(entry)
                }
            }
            ContextVideoCacheIndex(serverId, entries)
        }.getOrElse {
            file.delete()
            ContextVideoCacheIndex("", emptyList())
        }
    }

    fun write(root: File, index: ContextVideoCacheIndex) {
        root.mkdirs()
        val target = File(root, INDEX_FILE)
        val temporary = File(root, "$INDEX_FILE.tmp")
        val array = JSONArray()
        index.entries.forEach { entry ->
            array.put(
                JSONObject()
                    .put("videoId", entry.videoId)
                    .put("serverId", entry.serverId)
                    .put("version", entry.version)
                    .put("expectedVideoBytes", entry.expectedVideoBytes)
                    .put("actualVideoBytes", entry.actualVideoBytes)
                    .put("videoFileName", entry.videoFileName)
                    .put("subtitleFileName", entry.subtitleFileName)
                    .put("createdAt", entry.createdAt)
                    .put("downloadedAt", entry.downloadedAt)
                    .put("lastAccessedAt", entry.lastAccessedAt)
                    .put("retainUntil", entry.retainUntil)
                    .put("priority", entry.priority),
            )
        }
        val payload = JSONObject()
            .put("schemaVersion", SCHEMA_VERSION)
            .put("serverId", index.serverId)
            .put("entries", array)
            .toString()
        temporary.outputStream().use { output ->
            output.write(payload.toByteArray())
            output.fd.sync()
        }
        check(temporary.renameTo(target)) { "unable to commit context video cache index" }
    }

    private fun CachedContextVideo.isValid(): Boolean =
        videoId > 0L &&
            serverId.isNotEmpty() &&
            version.isNotEmpty() &&
            actualVideoBytes > 0L &&
            videoFileName.isSafeFileName() &&
            subtitleFileName.isSafeFileName()

    private fun String.isSafeFileName(): Boolean =
        isNotEmpty() && !contains('/') && !contains('\\') && !contains("..")

    private const val SCHEMA_VERSION = 1
    private const val INDEX_FILE = "index.json"
}
