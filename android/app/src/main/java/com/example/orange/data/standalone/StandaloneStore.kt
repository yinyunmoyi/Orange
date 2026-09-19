package com.example.orange.data.standalone

import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.DatabaseUtils
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

class StandaloneStore(context: Context) : SQLiteOpenHelper(
    context.applicationContext,
    DATABASE_NAME,
    null,
    DATABASE_VERSION,
) {
    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE standalone_favorites (
                client_id TEXT PRIMARY KEY,
                item_type TEXT NOT NULL,
                normalized_text TEXT NOT NULL,
                display_json TEXT NOT NULL DEFAULT '{}',
                translation TEXT NOT NULL DEFAULT '',
                created_at INTEGER NOT NULL,
                sync_state TEXT NOT NULL DEFAULT 'pending',
                server_id INTEGER,
                last_error TEXT,
                retry_count INTEGER NOT NULL DEFAULT 0,
                UNIQUE(item_type, normalized_text)
            )
            """.trimIndent(),
        )
        db.execSQL(
            """
            CREATE TABLE standalone_notes (
                item_type TEXT NOT NULL,
                normalized_text TEXT NOT NULL,
                note TEXT NOT NULL,
                revision INTEGER NOT NULL,
                synced_revision INTEGER NOT NULL DEFAULT 0,
                PRIMARY KEY(item_type, normalized_text)
            )
            """.trimIndent(),
        )
        db.execSQL(
            """
            CREATE TABLE standalone_contexts (
                event_id TEXT PRIMARY KEY,
                client_id TEXT NOT NULL,
                paragraph TEXT NOT NULL,
                selection_start INTEGER NOT NULL,
                selection_end INTEGER NOT NULL,
                confirmed INTEGER NOT NULL DEFAULT 0,
                FOREIGN KEY(client_id) REFERENCES standalone_favorites(client_id) ON DELETE CASCADE
            )
            """.trimIndent(),
        )
        db.execSQL(
            """
            CREATE TABLE standalone_actions (
                event_id TEXT PRIMARY KEY,
                client_id TEXT NOT NULL,
                action TEXT NOT NULL,
                source TEXT NOT NULL,
                metadata_json TEXT NOT NULL DEFAULT '{}',
                confirmed INTEGER NOT NULL DEFAULT 0,
                FOREIGN KEY(client_id) REFERENCES standalone_favorites(client_id) ON DELETE CASCADE
            )
            """.trimIndent(),
        )
        db.execSQL("CREATE INDEX standalone_favorites_sync_idx ON standalone_favorites(sync_state, created_at)")
        db.execSQL("CREATE INDEX standalone_contexts_client_idx ON standalone_contexts(client_id, confirmed)")
        db.execSQL("CREATE INDEX standalone_actions_client_idx ON standalone_actions(client_id, confirmed)")
    }

    override fun onConfigure(db: SQLiteDatabase) {
        super.onConfigure(db)
        db.setForeignKeyConstraintsEnabled(true)
        db.enableWriteAheadLogging()
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) = Unit

    fun upsertFavorite(favorite: StandaloneFavorite): StandaloneFavorite =
        writableDatabase.inTransaction {
            insertWithOnConflict(
                "standalone_favorites",
                null,
                ContentValues().apply {
                    put("client_id", favorite.clientId)
                    put("item_type", favorite.itemType.wireValue)
                    put("normalized_text", favorite.text)
                    put("display_json", favorite.displayJson)
                    put("translation", favorite.translation)
                    put("created_at", favorite.createdAt)
                    put("sync_state", favorite.syncState.wireValue)
                    put("retry_count", favorite.retryCount)
                },
                SQLiteDatabase.CONFLICT_IGNORE,
            )
            queryFavorite(this, favorite.itemType, favorite.text)
                ?: error("standalone favorite insert failed")
        }

    fun listFavorites(): List<StandaloneFavorite> =
        readableDatabase.query(
            "standalone_favorites",
            FAVORITE_COLUMNS,
            null,
            null,
            null,
            null,
            "created_at DESC",
        ).use { cursor ->
            buildList {
                while (cursor.moveToNext()) add(cursor.toStandaloneFavorite())
            }
        }

    fun findFavorite(type: StandaloneItemType, text: String): StandaloneFavorite? =
        queryFavorite(readableDatabase, type, normalizeStandaloneText(type, text))

    fun markFavoriteMapped(clientId: String, serverId: Long) {
        require(serverId > 0L)
        writableDatabase.update(
            "standalone_favorites",
            ContentValues().apply {
                put("sync_state", StandaloneSyncState.MAPPED.wireValue)
                put("server_id", serverId)
                putNull("last_error")
            },
            "client_id = ?",
            arrayOf(clientId),
        )
    }

    fun markFavoriteFailed(clientId: String, error: String) {
        writableDatabase.execSQL(
            """
            UPDATE standalone_favorites
            SET last_error = ?, retry_count = retry_count + 1
            WHERE client_id = ?
            """.trimIndent(),
            arrayOf(error.take(500), clientId),
        )
    }

    fun saveNote(type: StandaloneItemType, text: String, note: String): StandaloneNote {
        val normalized = normalizeStandaloneText(type, text)
        writableDatabase.inTransaction {
            val inserted = insertWithOnConflict(
                "standalone_notes",
                null,
                ContentValues().apply {
                    put("item_type", type.wireValue)
                    put("normalized_text", normalized)
                    put("note", note)
                    put("revision", 1L)
                    put("synced_revision", 0L)
                },
                SQLiteDatabase.CONFLICT_IGNORE,
            )
            if (inserted == -1L) {
                execSQL(
                    """
                    UPDATE standalone_notes
                    SET note = ?, revision = revision + 1
                    WHERE item_type = ? AND normalized_text = ?
                    """.trimIndent(),
                    arrayOf(note, type.wireValue, normalized),
                )
            }
        }
        return getNote(type, normalized) ?: error("standalone note save failed")
    }

    fun getNote(type: StandaloneItemType, text: String): StandaloneNote? {
        val normalized = normalizeStandaloneText(type, text)
        return readableDatabase.query(
            "standalone_notes",
            NOTE_COLUMNS,
            "item_type = ? AND normalized_text = ?",
            arrayOf(type.wireValue, normalized),
            null,
            null,
            null,
            "1",
        ).use { cursor -> if (cursor.moveToFirst()) cursor.toStandaloneNote() else null }
    }

    fun pendingNotes(): List<StandaloneNote> =
        readableDatabase.query(
            "standalone_notes",
            NOTE_COLUMNS,
            "revision > synced_revision",
            null,
            null,
            null,
            "revision ASC",
        ).use { cursor ->
            buildList { while (cursor.moveToNext()) add(cursor.toStandaloneNote()) }
        }

    fun confirmNote(type: StandaloneItemType, text: String, revision: Long) {
        writableDatabase.execSQL(
            """
            UPDATE standalone_notes
            SET synced_revision = ?
            WHERE item_type = ? AND normalized_text = ? AND revision = ?
            """.trimIndent(),
            arrayOf<Any>(revision, type.wireValue, normalizeStandaloneText(type, text), revision),
        )
    }

    fun insertContext(context: StandaloneContext) {
        writableDatabase.insertWithOnConflict(
            "standalone_contexts",
            null,
            ContentValues().apply {
                put("event_id", context.eventId)
                put("client_id", context.clientId)
                put("paragraph", context.paragraph)
                put("selection_start", context.selectionStart)
                put("selection_end", context.selectionEnd)
            },
            SQLiteDatabase.CONFLICT_IGNORE,
        )
    }

    fun pendingContexts(clientId: String): List<StandaloneContext> =
        readableDatabase.query(
            "standalone_contexts",
            CONTEXT_COLUMNS,
            "client_id = ? AND confirmed = 0",
            arrayOf(clientId),
            null,
            null,
            "rowid ASC",
        ).use { cursor ->
            buildList { while (cursor.moveToNext()) add(cursor.toStandaloneContext()) }
        }

    fun confirmContext(eventId: String) = confirmEvent("standalone_contexts", eventId)

    fun insertAction(action: StandaloneAction) {
        writableDatabase.insertWithOnConflict(
            "standalone_actions",
            null,
            ContentValues().apply {
                put("event_id", action.eventId)
                put("client_id", action.clientId)
                put("action", action.action)
                put("source", action.source)
                put("metadata_json", action.metadataJson)
            },
            SQLiteDatabase.CONFLICT_IGNORE,
        )
    }

    fun pendingActions(clientId: String): List<StandaloneAction> =
        readableDatabase.query(
            "standalone_actions",
            ACTION_COLUMNS,
            "client_id = ? AND confirmed = 0",
            arrayOf(clientId),
            null,
            null,
            "rowid ASC",
        ).use { cursor ->
            buildList { while (cursor.moveToNext()) add(cursor.toStandaloneAction()) }
        }

    fun confirmAction(eventId: String) = confirmEvent("standalone_actions", eventId)

    fun cleanupMappedFavorite(clientId: String): Boolean = writableDatabase.inTransaction {
        val favorite = queryFavoriteByClientId(this, clientId) ?: return@inTransaction false
        if (favorite.syncState != StandaloneSyncState.MAPPED) return@inTransaction false
        val pendingChildren = DatabaseUtils.longForQuery(
            this,
            """
            SELECT
              (SELECT COUNT(*) FROM standalone_contexts WHERE client_id = ? AND confirmed = 0) +
              (SELECT COUNT(*) FROM standalone_actions WHERE client_id = ? AND confirmed = 0)
            """.trimIndent(),
            arrayOf(clientId, clientId),
        )
        val note = queryNote(this, favorite.itemType, favorite.text)
        if (pendingChildren > 0L || (note != null && note.revision > note.syncedRevision)) {
            return@inTransaction false
        }
        delete(
            "standalone_notes",
            "item_type = ? AND normalized_text = ?",
            arrayOf(favorite.itemType.wireValue, favorite.text),
        )
        delete("standalone_favorites", "client_id = ?", arrayOf(clientId)) > 0
    }

    private fun confirmEvent(table: String, eventId: String) {
        require(table == "standalone_contexts" || table == "standalone_actions")
        writableDatabase.update(
            table,
            ContentValues().apply { put("confirmed", 1) },
            "event_id = ?",
            arrayOf(eventId),
        )
    }

    private fun queryFavorite(
        db: SQLiteDatabase,
        type: StandaloneItemType,
        normalizedText: String,
    ): StandaloneFavorite? = db.query(
        "standalone_favorites",
        FAVORITE_COLUMNS,
        "item_type = ? AND normalized_text = ?",
        arrayOf(type.wireValue, normalizedText),
        null,
        null,
        null,
        "1",
    ).use { cursor -> if (cursor.moveToFirst()) cursor.toStandaloneFavorite() else null }

    private fun queryFavoriteByClientId(db: SQLiteDatabase, clientId: String): StandaloneFavorite? =
        db.query(
            "standalone_favorites",
            FAVORITE_COLUMNS,
            "client_id = ?",
            arrayOf(clientId),
            null,
            null,
            null,
            "1",
        ).use { cursor -> if (cursor.moveToFirst()) cursor.toStandaloneFavorite() else null }

    private fun queryNote(
        db: SQLiteDatabase,
        type: StandaloneItemType,
        normalizedText: String,
    ): StandaloneNote? = db.query(
        "standalone_notes",
        NOTE_COLUMNS,
        "item_type = ? AND normalized_text = ?",
        arrayOf(type.wireValue, normalizedText),
        null,
        null,
        null,
        "1",
    ).use { cursor -> if (cursor.moveToFirst()) cursor.toStandaloneNote() else null }

    private fun Cursor.toStandaloneFavorite() = StandaloneFavorite(
        clientId = getString(getColumnIndexOrThrow("client_id")),
        itemType = StandaloneItemType.fromWire(getString(getColumnIndexOrThrow("item_type"))),
        text = getString(getColumnIndexOrThrow("normalized_text")),
        displayJson = getString(getColumnIndexOrThrow("display_json")),
        translation = getString(getColumnIndexOrThrow("translation")),
        createdAt = getLong(getColumnIndexOrThrow("created_at")),
        syncState = StandaloneSyncState.fromWire(getString(getColumnIndexOrThrow("sync_state"))),
        serverId = getColumnIndexOrThrow("server_id").let { if (isNull(it)) null else getLong(it) },
        lastError = getColumnIndexOrThrow("last_error").let { if (isNull(it)) null else getString(it) },
        retryCount = getInt(getColumnIndexOrThrow("retry_count")),
    )

    private fun Cursor.toStandaloneNote() = StandaloneNote(
        itemType = StandaloneItemType.fromWire(getString(getColumnIndexOrThrow("item_type"))),
        normalizedText = getString(getColumnIndexOrThrow("normalized_text")),
        note = getString(getColumnIndexOrThrow("note")),
        revision = getLong(getColumnIndexOrThrow("revision")),
        syncedRevision = getLong(getColumnIndexOrThrow("synced_revision")),
    )

    private fun Cursor.toStandaloneContext() = StandaloneContext(
        eventId = getString(getColumnIndexOrThrow("event_id")),
        clientId = getString(getColumnIndexOrThrow("client_id")),
        paragraph = getString(getColumnIndexOrThrow("paragraph")),
        selectionStart = getInt(getColumnIndexOrThrow("selection_start")),
        selectionEnd = getInt(getColumnIndexOrThrow("selection_end")),
    )

    private fun Cursor.toStandaloneAction() = StandaloneAction(
        eventId = getString(getColumnIndexOrThrow("event_id")),
        clientId = getString(getColumnIndexOrThrow("client_id")),
        action = getString(getColumnIndexOrThrow("action")),
        source = getString(getColumnIndexOrThrow("source")),
        metadataJson = getString(getColumnIndexOrThrow("metadata_json")),
    )

    private inline fun <T> SQLiteDatabase.inTransaction(block: SQLiteDatabase.() -> T): T {
        beginTransaction()
        try {
            return block().also { setTransactionSuccessful() }
        } finally {
            endTransaction()
        }
    }

    private companion object {
        const val DATABASE_NAME = "standalone.db"
        const val DATABASE_VERSION = 1
        val FAVORITE_COLUMNS = arrayOf(
            "client_id", "item_type", "normalized_text", "display_json", "translation",
            "created_at", "sync_state", "server_id", "last_error", "retry_count",
        )
        val NOTE_COLUMNS = arrayOf(
            "item_type", "normalized_text", "note", "revision", "synced_revision",
        )
        val CONTEXT_COLUMNS = arrayOf(
            "event_id", "client_id", "paragraph", "selection_start", "selection_end",
        )
        val ACTION_COLUMNS = arrayOf(
            "event_id", "client_id", "action", "source", "metadata_json",
        )
    }
}
