package com.example.orange.data.contextsync

import org.json.JSONObject

data class ContextVideoSyncItem(
    val videoId: Long,
    val contextId: Long,
    val itemType: String,
    val itemId: Long,
    val durationMs: Long,
    val fileSize: Long,
    val version: String,
    val videoUrl: String,
    val subtitleUrl: String,
    val createdAt: String,
    val dueAt: String,
    val priority: Int,
)

data class ContextVideoSyncManifest(
    val serverId: String,
    val generatedAt: String,
    val horizonEnd: String,
    val items: List<ContextVideoSyncItem>,
)

internal fun parseContextVideoSyncManifest(
    root: JSONObject,
    expectedServerId: String,
): ContextVideoSyncManifest {
    val data = root.optJSONObject("data")
        ?: error(root.optString("msg", "response missing data"))
    val serverId = data.optString("serverId").trim()
    require(serverId.isNotEmpty() && serverId == expectedServerId) {
        "context sync server mismatch"
    }
    val items = buildList {
        val array = data.optJSONArray("items") ?: return@buildList
        for (index in 0 until array.length()) {
            val item = array.optJSONObject(index) ?: continue
            val parsed = ContextVideoSyncItem(
                videoId = item.optLong("videoId"),
                contextId = item.optLong("contextId"),
                itemType = item.optString("itemType").trim(),
                itemId = item.optLong("itemId"),
                durationMs = item.optLong("durationMs"),
                fileSize = item.optLong("fileSize"),
                version = item.optString("version").trim(),
                videoUrl = item.optString("videoUrl").trim(),
                subtitleUrl = item.optString("subtitleUrl").trim(),
                createdAt = item.optString("createdAt").trim(),
                dueAt = item.optString("dueAt").trim(),
                priority = item.optInt("priority", 2).coerceIn(0, 2),
            )
            if (parsed.isValid()) add(parsed)
        }
    }
    return ContextVideoSyncManifest(
        serverId = serverId,
        generatedAt = data.optString("generatedAt").trim(),
        horizonEnd = data.optString("horizonEnd").trim(),
        items = items,
    )
}

private fun ContextVideoSyncItem.isValid(): Boolean =
    videoId > 0L &&
        contextId > 0L &&
        itemId > 0L &&
        itemType in setOf("word", "phrase") &&
        durationMs > 0L &&
        fileSize > 0L &&
        version.isNotEmpty() &&
        videoUrl.isSafeApiPath() &&
        subtitleUrl.isSafeApiPath()

internal fun String.isSafeApiPath(): Boolean =
    startsWith("/api/v1/") &&
        !startsWith("//") &&
        !contains("://") &&
        !contains("..")
