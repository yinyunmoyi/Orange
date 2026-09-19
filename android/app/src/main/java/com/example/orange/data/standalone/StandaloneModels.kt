package com.example.orange.data.standalone

import com.example.orange.data.word.FavoriteItem
import com.example.orange.data.word.SentenceFavorite
import java.security.MessageDigest
import java.util.Locale

enum class StandaloneItemType(val wireValue: String) {
    WORD("word"),
    PHRASE("phrase"),
    SENTENCE("sentence");

    companion object {
        fun fromWire(value: String): StandaloneItemType =
            entries.firstOrNull { it.wireValue == value }
                ?: throw IllegalArgumentException("invalid standalone item type")
    }
}

enum class StandaloneSyncState(val wireValue: String) {
    PENDING("pending"),
    MAPPED("mapped");

    companion object {
        fun fromWire(value: String): StandaloneSyncState =
            entries.firstOrNull { it.wireValue == value } ?: PENDING
    }
}

data class StandaloneFavorite(
    val clientId: String,
    val itemType: StandaloneItemType,
    val text: String,
    val displayJson: String,
    val translation: String,
    val createdAt: Long,
    val syncState: StandaloneSyncState,
    val serverId: Long?,
    val lastError: String?,
    val retryCount: Int,
)

data class StandaloneNote(
    val itemType: StandaloneItemType,
    val normalizedText: String,
    val note: String,
    val revision: Long,
    val syncedRevision: Long,
)

data class StandaloneContext(
    val eventId: String,
    val clientId: String,
    val paragraph: String,
    val selectionStart: Int,
    val selectionEnd: Int,
)

data class StandaloneAction(
    val eventId: String,
    val clientId: String,
    val action: String,
    val source: String,
    val metadataJson: String,
)

data class StandaloneSnapshot(
    val favorites: List<StandaloneFavorite> = emptyList(),
) {
    val pendingCount: Int get() = favorites.count { it.syncState == StandaloneSyncState.PENDING }

    val wordItems: List<FavoriteItem>
        get() = favorites.asSequence()
            .filter { it.itemType != StandaloneItemType.SENTENCE }
            .sortedByDescending { it.createdAt }
            .map {
                FavoriteItem(
                    itemType = it.itemType.wireValue,
                    itemId = it.serverId ?: 0L,
                    text = it.text,
                    createdAt = it.createdAt.toString(),
                    topMeaningPos = "",
                    topMeaningText = standaloneTopMeaning(it.displayJson),
                    clientId = it.clientId,
                    syncState = it.syncState.wireValue,
                )
            }
            .toList()

    val sentenceItems: List<SentenceFavorite>
        get() = favorites.asSequence()
            .filter { it.itemType == StandaloneItemType.SENTENCE }
            .sortedByDescending { it.createdAt }
            .map {
                SentenceFavorite(
                    id = it.serverId ?: 0L,
                    sentence = it.text,
                    translation = it.translation,
                    createdAt = it.createdAt.toString(),
                    clientId = it.clientId,
                    syncState = it.syncState.wireValue,
                )
            }
            .toList()
}

fun normalizeStandaloneText(type: StandaloneItemType, raw: String): String {
    val collapsed = raw.trim().replace(Regex("\\s+"), " ")
    require(collapsed.isNotEmpty()) { "text is blank" }
    return when (type) {
        StandaloneItemType.WORD -> collapsed.lowercase(Locale.ROOT).also {
            require(it.length <= 64 && WORD_PATTERN.matches(it)) { "invalid word" }
        }
        StandaloneItemType.PHRASE -> collapsed.lowercase(Locale.ROOT).also {
            require(
                it.length <= 80 &&
                    PHRASE_PATTERN.matches(it) &&
                    !BROKEN_HYPHEN_WRAP_PATTERN.containsMatchIn(it),
            ) { "invalid phrase" }
        }
        StandaloneItemType.SENTENCE -> collapsed.also {
            require(
                it.codePointCount(0, it.length) <= 500 && SENTENCE_PATTERN.matches(it),
            ) { "invalid sentence" }
        }
    }
}

fun standaloneSentenceKey(sentence: String): String {
    val normalized = sentence.trim().replace(Regex("\\s+"), " ")
    return MessageDigest.getInstance("SHA-256")
        .digest(normalized.toByteArray(Charsets.UTF_8))
        .joinToString("") { "%02x".format(it) }
}

internal fun mergeStandaloneWords(
    remote: List<FavoriteItem>,
    local: List<FavoriteItem>,
): List<FavoriteItem> {
    val remoteKeys = remote.mapTo(hashSetOf()) {
        "${it.itemType}:${normalizeStandaloneText(StandaloneItemType.fromWire(it.itemType), it.text)}"
    }
    return local.filter {
        "${it.itemType}:${normalizeStandaloneText(StandaloneItemType.fromWire(it.itemType), it.text)}" !in remoteKeys
    } + remote
}

internal fun mergeStandaloneSentences(
    remote: List<SentenceFavorite>,
    local: List<SentenceFavorite>,
): List<SentenceFavorite> {
    val remoteKeys = remote.mapTo(hashSetOf()) { standaloneSentenceKey(it.sentence) }
    return local.filter { standaloneSentenceKey(it.sentence) !in remoteKeys } + remote
}

private fun standaloneTopMeaning(displayJson: String): String =
    runCatching {
        org.json.JSONObject(displayJson)
            .optJSONArray("meanings")
            ?.optJSONObject(0)
            ?.optString("meaning", "")
            .orEmpty()
    }.getOrDefault("")

private val WORD_PATTERN = Regex("^[a-z][a-z'-]*$")
private val PHRASE_PATTERN = Regex("^[a-z][a-z'-]*(?: [a-z][a-z'-]*){1,7}$")
private val BROKEN_HYPHEN_WRAP_PATTERN = Regex("(?:^| )[a-z]+-[a-z] [a-z]{2,}(?: |$)")
private val SENTENCE_PATTERN = Regex("^[\\x20-\\x7E\\p{IsLatin}\\p{M}]+$")
