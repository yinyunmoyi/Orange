package com.example.orange.data.standalone

import com.example.orange.data.mode.AppMode
import com.example.orange.data.mode.AppModeRepository
import com.example.orange.data.offline.OfflineLlmClient
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
    val mode: AppMode,
    private val offlineClient: OfflineLlmClient,
) {
    val standalone: Boolean
        get() = mode.usesLocalData

    suspend fun prepareLookup(
        text: String,
        isPhrase: Boolean,
        context: String,
        start: Int,
        end: Int,
    ): Result<Unit> = if (mode == AppMode.OFFLINE) {
        offlineClient.prepareLookup(text, isPhrase, context, start, end)
    } else {
        Result.success(Unit)
    }

    suspend fun lookup(word: String): Result<LookupData> =
        when (mode) {
            AppMode.NORMAL -> WordApi.lookup(word)
            AppMode.STANDALONE -> StandaloneLlmClient.lookup(word)
            AppMode.OFFLINE -> offlineClient.lookup()
        }

    suspend fun meaning(word: String): Result<MeaningData> =
        when (mode) {
            AppMode.NORMAL -> WordApi.meaning(word)
            AppMode.STANDALONE -> StandaloneLlmClient.meaning(word)
            AppMode.OFFLINE -> offlineClient.meaning()
        }

    suspend fun phrase(phrase: String): Result<MeaningData> =
        when (mode) {
            AppMode.NORMAL -> WordApi.phrase(phrase)
            AppMode.STANDALONE -> StandaloneLlmClient.phrase(phrase)
            AppMode.OFFLINE -> offlineClient.meaning()
        }

    suspend fun explain(word: String, context: String, start: Int, end: Int): Result<ExplainData> =
        when (mode) {
            AppMode.NORMAL -> WordApi.explain(word, context, start, end)
            AppMode.STANDALONE -> StandaloneLlmClient.explain(word, context, start, end)
            AppMode.OFFLINE -> offlineClient.explain()
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
        when (mode) {
            AppMode.NORMAL -> WordApi.analyzeSentence(sentence)
            AppMode.STANDALONE -> StandaloneLlmClient.analyzeSentence(sentence)
            AppMode.OFFLINE -> offlineClient.analyze(sentence)
        }

    suspend fun translateSentenceStream(
        sentence: String,
        onDelta: (String) -> Unit,
        onDone: () -> Unit,
        onError: (String) -> Unit,
    ) {
        when (mode) {
            AppMode.NORMAL -> WordApi.translateSentenceStream(sentence, onDelta, onDone, onError)
            AppMode.STANDALONE ->
                StandaloneLlmClient.translateSentenceStream(sentence, onDelta, onDone, onError)
            AppMode.OFFLINE -> offlineClient.translate(sentence).fold(
                onSuccess = {
                    onDelta(it)
                    onDone()
                },
                onFailure = { onError(it.message ?: it.javaClass.simpleName) },
            )
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
            ReaderDataRepository(AppModeRepository.current(), OfflineLlmClient())
    }
}
