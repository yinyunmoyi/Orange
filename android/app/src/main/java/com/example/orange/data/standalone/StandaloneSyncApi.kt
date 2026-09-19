package com.example.orange.data.standalone

import com.example.orange.data.word.WordServiceConfig
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

internal data class StandaloneSyncMapping(
    val clientId: String,
    val itemType: StandaloneItemType,
    val itemId: Long,
)

internal object StandaloneSyncApi {
    fun candidateOrigins(localOrigin: String?): List<String> =
        listOfNotNull(localOrigin?.trim()?.trimEnd('/')?.takeIf(String::isNotEmpty), WordServiceConfig.origin)
            .distinct()

    fun syncFavorite(origin: String, favorite: StandaloneFavorite): StandaloneSyncMapping {
        val body = JSONObject()
            .put("clientId", favorite.clientId)
            .put("itemType", favorite.itemType.wireValue)
            .put("text", favorite.text)
        if (favorite.itemType == StandaloneItemType.SENTENCE) {
            body.put("translation", favorite.translation)
        }
        val data = request(
            "POST",
            "$origin/api/v1/standalone-sync/favorites",
            body,
            readTimeoutMs = 180_000,
        ).optJSONObject("data") ?: throw IOException("sync favorite response missing data")
        val itemId = data.optLong("itemId", 0L)
        if (itemId <= 0L || data.optString("clientId") != favorite.clientId) {
            throw IOException("sync favorite response invalid")
        }
        return StandaloneSyncMapping(
            clientId = favorite.clientId,
            itemType = StandaloneItemType.fromWire(data.optString("itemType")),
            itemId = itemId,
        )
    }

    fun syncNote(origin: String, favorite: StandaloneFavorite, note: StandaloneNote) {
        request(
            "PUT",
            "$origin/api/v1/notes",
            JSONObject()
                .put("itemType", favorite.itemType.wireValue)
                .put("text", favorite.text)
                .put("note", note.note),
        )
    }

    fun syncContext(
        origin: String,
        favorite: StandaloneFavorite,
        context: StandaloneContext,
    ) {
        request(
            "POST",
            "$origin/api/v1/contexts/tasks",
            JSONObject()
                .put("itemType", favorite.itemType.wireValue)
                .put("itemId", requireNotNull(favorite.serverId))
                .put("paragraph", context.paragraph)
                .put("selectionStart", context.selectionStart)
                .put("selectionEnd", context.selectionEnd)
                .put("source", "android_reader"),
            readTimeoutMs = 180_000,
        )
    }

    fun syncAction(
        origin: String,
        favorite: StandaloneFavorite,
        action: StandaloneAction,
    ) {
        request(
            "POST",
            "$origin/api/v1/favorites/${favorite.itemType.wireValue}/${requireNotNull(favorite.serverId)}/actions",
            JSONObject()
                .put("eventId", action.eventId)
                .put("action", action.action)
                .put("source", action.source)
                .put("metadata", JSONObject(action.metadataJson)),
        )
    }

    private fun request(
        method: String,
        url: String,
        body: JSONObject,
        readTimeoutMs: Int = 30_000,
    ): JSONObject {
        val connection = URL(url).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = method
            connection.doOutput = true
            connection.doInput = true
            connection.connectTimeout = 1_500
            connection.readTimeout = readTimeoutMs
            connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Connection", "close")
            connection.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            val code = connection.responseCode
            val text = if (code in 200..299) {
                connection.inputStream.use { it.reader(Charsets.UTF_8).readText() }
            } else {
                connection.errorStream?.use { it.reader(Charsets.UTF_8).readText() }.orEmpty()
            }
            if (code !in 200..299) throw IOException("HTTP $code ${text.take(200)}")
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        } finally {
            connection.disconnect()
        }
    }
}
