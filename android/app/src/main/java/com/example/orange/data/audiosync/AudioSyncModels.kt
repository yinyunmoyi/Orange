package com.example.orange.data.audiosync

import org.json.JSONObject

data class AudioSyncItem(
    val itemType: String,
    val itemId: Long,
    val audioId: Long,
    val accent: String,
    val fileSize: Long,
    val version: String,
    val audioUrl: String,
    val playbackUrl: String,
)

data class AudioSyncManifest(
    val serverId: String,
    val generatedAt: String,
    val items: List<AudioSyncItem>,
)

internal fun parseAudioSyncManifest(
    root: JSONObject,
    expectedServerId: String,
): AudioSyncManifest {
    val data = root.optJSONObject("data")
        ?: error(root.optString("msg", "response missing data"))
    val serverId = data.optString("serverId").trim()
    require(serverId.isNotEmpty() && serverId == expectedServerId) {
        "audio sync server mismatch"
    }
    val items = buildList {
        val array = data.optJSONArray("items") ?: return@buildList
        for (index in 0 until array.length()) {
            val value = array.optJSONObject(index) ?: continue
            val item = AudioSyncItem(
                itemType = value.optString("itemType").trim(),
                itemId = value.optLong("itemId"),
                audioId = value.optLong("audioId"),
                accent = value.optString("accent").trim(),
                fileSize = value.optLong("fileSize"),
                version = value.optString("version").trim(),
                audioUrl = value.optString("audioUrl").trim(),
                playbackUrl = value.optString("playbackUrl").trim(),
            )
            if (item.isValid()) add(item)
        }
    }
    return AudioSyncManifest(
        serverId = serverId,
        generatedAt = data.optString("generatedAt").trim(),
        items = items,
    )
}

private fun AudioSyncItem.isValid(): Boolean =
    itemType in setOf("word", "phrase") &&
        itemId > 0L &&
        ((itemType == "word" && audioId > 0L) || (itemType == "phrase" && audioId == 0L)) &&
        (itemType == "phrase" || accent.isNotEmpty()) &&
        fileSize > 0L &&
        version.matches(Regex("[a-f0-9]{64}")) &&
        audioUrl.isSafeAudioApiPath() &&
        playbackUrl.isSafeAudioApiPath()

internal fun String.isSafeAudioApiPath(): Boolean =
    startsWith("/api/v1/") &&
        !startsWith("//") &&
        !contains("://") &&
        !contains("..")
