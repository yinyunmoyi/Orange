package com.example.orange.data.audiosync

import org.json.JSONArray
import org.json.JSONObject
import java.io.File

internal data class CachedAudio(
    val itemType: String,
    val itemId: Long,
    val audioId: Long,
    val accent: String,
    val version: String,
    val playbackUrl: String,
    val fileName: String,
    val actualBytes: Long,
)

internal data class AudioCacheIndex(
    val serverId: String,
    val entries: List<CachedAudio>,
)

internal object AudioCacheIndexStore {
    fun read(root: File): AudioCacheIndex {
        val file = File(root, INDEX_FILE)
        if (!file.isFile) return AudioCacheIndex("", emptyList())
        return runCatching {
            val json = JSONObject(file.readText())
            require(json.optInt("schemaVersion") == SCHEMA_VERSION)
            val entries = buildList {
                val array = json.optJSONArray("entries") ?: JSONArray()
                for (index in 0 until array.length()) {
                    val value = array.optJSONObject(index) ?: continue
                    val entry = CachedAudio(
                        itemType = value.optString("itemType"),
                        itemId = value.optLong("itemId"),
                        audioId = value.optLong("audioId"),
                        accent = value.optString("accent"),
                        version = value.optString("version"),
                        playbackUrl = value.optString("playbackUrl"),
                        fileName = value.optString("fileName"),
                        actualBytes = value.optLong("actualBytes"),
                    )
                    if (entry.isValid()) add(entry)
                }
            }
            AudioCacheIndex(json.optString("serverId"), entries)
        }.getOrElse {
            file.delete()
            AudioCacheIndex("", emptyList())
        }
    }

    fun write(root: File, index: AudioCacheIndex) {
        root.mkdirs()
        val temporary = File(root, "$INDEX_FILE.tmp")
        val target = File(root, INDEX_FILE)
        val entries = JSONArray()
        index.entries.forEach { entry ->
            entries.put(
                JSONObject()
                    .put("itemType", entry.itemType)
                    .put("itemId", entry.itemId)
                    .put("audioId", entry.audioId)
                    .put("accent", entry.accent)
                    .put("version", entry.version)
                    .put("playbackUrl", entry.playbackUrl)
                    .put("fileName", entry.fileName)
                    .put("actualBytes", entry.actualBytes),
            )
        }
        val payload = JSONObject()
            .put("schemaVersion", SCHEMA_VERSION)
            .put("serverId", index.serverId)
            .put("entries", entries)
            .toString()
        temporary.outputStream().use { output ->
            output.write(payload.toByteArray())
            output.fd.sync()
        }
        check(temporary.renameTo(target)) { "unable to commit audio cache index" }
    }

    private fun CachedAudio.isValid(): Boolean =
        itemType in setOf("word", "phrase") &&
            itemId > 0L &&
            audioId != 0L &&
            version.isNotEmpty() &&
            playbackUrl.isSafeAudioApiPath() &&
            fileName.isSafeFileName() &&
            actualBytes > 0L

    private fun String.isSafeFileName(): Boolean =
        isNotEmpty() && !contains('/') && !contains('\\') && !contains("..")

    private const val SCHEMA_VERSION = 1
    private const val INDEX_FILE = "index.json"
}
