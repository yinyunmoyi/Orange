package com.example.orange.data.standalone

import android.content.Context
import com.example.orange.data.word.LookupData
import com.example.orange.data.word.MeaningData
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.util.UUID

object StandaloneRepository {
    private lateinit var localStore: StandaloneStore
    private lateinit var preferences: StandalonePreferences
    private val mutex = Mutex()
    private val mutableSnapshot = MutableStateFlow(StandaloneSnapshot())
    private val mutableSyncInProgress = MutableStateFlow(false)

    val snapshot: StateFlow<StandaloneSnapshot> = mutableSnapshot.asStateFlow()
    val syncInProgress: StateFlow<Boolean> = mutableSyncInProgress.asStateFlow()
    val modeEnabled: StateFlow<Boolean>
        get() = preferences.enabled

    fun init(context: Context) {
        if (::localStore.isInitialized) return
        synchronized(this) {
            if (::localStore.isInitialized) return
            localStore = StandaloneStore(context.applicationContext)
            preferences = StandalonePreferences(context.applicationContext)
            refreshBlocking()
        }
    }

    fun isEnabled(): Boolean {
        checkInitialized()
        return preferences.isEnabled()
    }

    fun setEnabled(enabled: Boolean) {
        checkInitialized()
        preferences.setEnabled(enabled)
    }

    suspend fun favoriteWord(
        word: String,
        english: LookupData?,
        chinese: MeaningData?,
    ): StandaloneFavorite = saveFavorite(
        type = StandaloneItemType.WORD,
        text = word,
        displayJson = buildWordDisplayJson(english, chinese),
    )

    suspend fun favoritePhrase(
        phrase: String,
        meaning: MeaningData?,
    ): StandaloneFavorite = saveFavorite(
        type = StandaloneItemType.PHRASE,
        text = phrase,
        displayJson = buildMeaningDisplayJson(meaning),
    )

    suspend fun favoriteSentence(
        sentence: String,
        translation: String,
    ): StandaloneFavorite {
        val normalizedTranslation = translation.trim()
        require(normalizedTranslation.isNotEmpty()) { "translation is blank" }
        require(normalizedTranslation.codePointCount(0, normalizedTranslation.length) <= 2000) {
            "translation is too long"
        }
        return saveFavorite(
            type = StandaloneItemType.SENTENCE,
            text = sentence,
            translation = normalizedTranslation,
        )
    }

    suspend fun favoriteStatus(type: StandaloneItemType, text: String): StandaloneFavorite? =
        withContext(Dispatchers.IO) {
            checkInitialized()
            localStore.findFavorite(type, text)
        }

    suspend fun getNote(type: StandaloneItemType, text: String): String = withContext(Dispatchers.IO) {
        checkInitialized()
        localStore.getNote(type, text)?.note.orEmpty()
    }

    suspend fun saveNote(type: StandaloneItemType, text: String, note: String): StandaloneNote =
        withContext(Dispatchers.IO) {
            checkInitialized()
            localStore.saveNote(type, text, note)
        }

    suspend fun recordContext(
        clientId: String,
        paragraph: String,
        selectionStart: Int,
        selectionEnd: Int,
        eventId: String = UUID.randomUUID().toString(),
    ) = withContext(Dispatchers.IO) {
        checkInitialized()
        require(paragraph.isNotBlank())
        require(selectionStart >= 0 && selectionEnd > selectionStart && selectionEnd <= paragraph.length)
        localStore.insertContext(
            StandaloneContext(
                eventId = eventId,
                clientId = clientId,
                paragraph = paragraph,
                selectionStart = selectionStart,
                selectionEnd = selectionEnd,
            ),
        )
    }

    suspend fun recordAction(
        clientId: String,
        eventId: String,
        action: String,
        source: String,
        metadata: JSONObject = JSONObject(),
    ) = withContext(Dispatchers.IO) {
        checkInitialized()
        localStore.insertAction(
            StandaloneAction(
                eventId = eventId,
                clientId = clientId,
                action = action,
                source = source,
                metadataJson = metadata.toString(),
            ),
        )
    }

    internal fun store(): StandaloneStore {
        checkInitialized()
        return localStore
    }

    internal fun syncPreferences(): StandalonePreferences {
        checkInitialized()
        return preferences
    }

    internal suspend fun refresh() = withContext(Dispatchers.IO) {
        checkInitialized()
        mutex.withLock { refreshBlocking() }
    }

    internal fun setSyncInProgress(inProgress: Boolean) {
        mutableSyncInProgress.value = inProgress
    }

    private suspend fun saveFavorite(
        type: StandaloneItemType,
        text: String,
        displayJson: String = "{}",
        translation: String = "",
    ): StandaloneFavorite = withContext(Dispatchers.IO) {
        checkInitialized()
        val normalized = normalizeStandaloneText(type, text)
        mutex.withLock {
            val saved = localStore.upsertFavorite(
                StandaloneFavorite(
                    clientId = UUID.randomUUID().toString(),
                    itemType = type,
                    text = normalized,
                    displayJson = displayJson,
                    translation = translation,
                    createdAt = System.currentTimeMillis(),
                    syncState = StandaloneSyncState.PENDING,
                    serverId = null,
                    lastError = null,
                    retryCount = 0,
                ),
            )
            refreshBlocking()
            saved
        }
    }

    private fun refreshBlocking() {
        mutableSnapshot.value = StandaloneSnapshot(
            favorites = localStore.listFavorites().filter {
                it.syncState == StandaloneSyncState.PENDING
            },
        )
    }

    private fun checkInitialized() {
        check(::localStore.isInitialized) { "StandaloneRepository.init must be called first" }
    }
}

private fun buildWordDisplayJson(english: LookupData?, chinese: MeaningData?): String =
    JSONObject().apply {
        put("word", english?.word.orEmpty())
        put("phonetic", english?.phonetic.orEmpty())
        put(
            "definitions",
            JSONArray().apply {
                english?.definitions?.forEach {
                    put(JSONObject().put("partOfSpeech", it.pos).put("definition", it.text))
                }
            },
        )
        put(
            "meanings",
            JSONArray().apply {
                chinese?.meanings?.forEach {
                    put(JSONObject().put("partOfSpeech", it.pos).put("meaning", it.meaning))
                }
            },
        )
    }.toString()

private fun buildMeaningDisplayJson(meaning: MeaningData?): String =
    JSONObject().apply {
        put(
            "meanings",
            JSONArray().apply {
                meaning?.meanings?.forEach {
                    put(JSONObject().put("partOfSpeech", it.pos).put("meaning", it.meaning))
                }
            },
        )
    }.toString()
