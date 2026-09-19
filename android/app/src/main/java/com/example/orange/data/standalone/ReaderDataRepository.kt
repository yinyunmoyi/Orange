package com.example.orange.data.standalone

import com.example.orange.data.word.ExplainData
import com.example.orange.data.word.FavoriteStatus
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import com.example.orange.data.word.NoteData
import com.example.orange.data.word.SentenceAnalysis
import com.example.orange.data.word.SentenceFavoriteStatus
import com.example.orange.data.word.WordApi
import org.json.JSONObject

data class FavoriteReference(
    val favorited: Boolean,
    val serverId: Long? = null,
    val clientId: String? = null,
)

class ReaderDataRepository private constructor(
    val standalone: Boolean,
) {
    suspend fun lookup(word: String): Result<LookupData> =
        if (standalone) StandaloneLlmClient.lookup(word) else WordApi.lookup(word)

    suspend fun meaning(word: String): Result<MeaningData> =
        if (standalone) StandaloneLlmClient.meaning(word) else WordApi.meaning(word)

    suspend fun phrase(phrase: String): Result<MeaningData> =
        if (standalone) StandaloneLlmClient.phrase(phrase) else WordApi.phrase(phrase)

    suspend fun explain(word: String, context: String, start: Int, end: Int): Result<ExplainData> =
        if (standalone) {
            StandaloneLlmClient.explain(word, context, start, end)
        } else {
            WordApi.explain(word, context, start, end)
        }

    suspend fun favoriteStatus(itemType: StandaloneItemType, text: String): Result<FavoriteReference> {
        if (standalone) {
            return runCatching {
                val favorite = StandaloneRepository.favoriteStatus(itemType, text)
                FavoriteReference(
                    favorited = favorite != null,
                    clientId = favorite?.clientId,
                )
            }
        }
        val remote: Result<FavoriteStatus> = if (itemType == StandaloneItemType.PHRASE) {
            WordApi.phraseFavoriteStatus(text)
        } else {
            WordApi.favoriteStatus(text)
        }
        return remote.map {
            FavoriteReference(
                favorited = it.favorited,
                serverId = it.itemId.takeIf { id -> id > 0L },
            )
        }
    }

    suspend fun favoriteWord(
        word: String,
        english: LookupData?,
        chinese: MeaningData?,
    ): Result<FavoriteReference> = if (standalone) {
        runCatching {
            val favorite = StandaloneRepository.favoriteWord(word, english, chinese)
            FavoriteReference(true, clientId = favorite.clientId)
        }
    } else {
        WordApi.favorite(word, english, chinese).map {
            FavoriteReference(true, serverId = it)
        }
    }

    suspend fun favoritePhrase(
        phrase: String,
        meaning: MeaningData,
    ): Result<FavoriteReference> = if (standalone) {
        runCatching {
            val favorite = StandaloneRepository.favoritePhrase(phrase, meaning)
            FavoriteReference(true, clientId = favorite.clientId)
        }
    } else {
        WordApi.favoritePhrase(phrase, meaning).map {
            FavoriteReference(true, serverId = it)
        }
    }

    suspend fun getNote(type: StandaloneItemType, text: String): Result<NoteData> =
        if (standalone) {
            runCatching {
                NoteData(type.wireValue, text, StandaloneRepository.getNote(type, text), "")
            }
        } else {
            WordApi.getNote(type.wireValue, text)
        }

    suspend fun recordAction(
        type: StandaloneItemType,
        reference: FavoriteReference,
        eventId: String,
        action: String,
        source: String,
        metadata: JSONObject = JSONObject(),
    ): Result<Unit> = if (standalone) {
        runCatching {
            StandaloneRepository.recordAction(
                clientId = requireNotNull(reference.clientId),
                eventId = eventId,
                action = action,
                source = source,
                metadata = metadata,
            )
        }
    } else {
        WordApi.recordFavoriteAction(
            itemType = type.wireValue,
            itemId = requireNotNull(reference.serverId),
            eventId = eventId,
            action = action,
            source = source,
            metadata = metadata,
        ).map { Unit }
    }

    suspend fun recordContext(
        type: StandaloneItemType,
        reference: FavoriteReference,
        paragraph: String,
        start: Int,
        end: Int,
    ): Result<Unit> = if (standalone) {
        runCatching {
            StandaloneRepository.recordContext(
                clientId = requireNotNull(reference.clientId),
                paragraph = paragraph,
                selectionStart = start,
                selectionEnd = end,
            )
        }
    } else {
        WordApi.createContextTask(
            itemType = type.wireValue,
            itemId = requireNotNull(reference.serverId),
            paragraph = paragraph,
            selectionStart = start,
            selectionEnd = end,
        )
    }

    suspend fun analyzeSentence(sentence: String): Result<SentenceAnalysis> =
        if (standalone) StandaloneLlmClient.analyzeSentence(sentence) else WordApi.analyzeSentence(sentence)

    suspend fun translateSentenceStream(
        sentence: String,
        onDelta: (String) -> Unit,
        onDone: () -> Unit,
        onError: (String) -> Unit,
    ) {
        if (standalone) {
            StandaloneLlmClient.translateSentenceStream(sentence, onDelta, onDone, onError)
        } else {
            WordApi.translateSentenceStream(sentence, onDelta, onDone, onError)
        }
    }

    suspend fun sentenceFavoriteStatus(sentence: String): Result<SentenceFavoriteStatus> =
        if (standalone) {
            runCatching {
                val favorite = StandaloneRepository.favoriteStatus(
                    StandaloneItemType.SENTENCE,
                    sentence,
                )
                SentenceFavoriteStatus(favorite != null, 0L)
            }
        } else {
            WordApi.sentenceFavoriteStatus(sentence)
        }

    suspend fun favoriteSentence(sentence: String, translation: String): Result<Unit> =
        if (standalone) {
            runCatching {
                StandaloneRepository.favoriteSentence(sentence, translation)
                Unit
            }
        } else {
            WordApi.favoriteSentence(sentence, translation).map { Unit }
        }

    companion object {
        fun capture(): ReaderDataRepository =
            ReaderDataRepository(StandaloneRepository.isEnabled())
    }
}
