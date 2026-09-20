package com.example.orange.data

import android.content.Context
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.ui.graphics.Color
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

data class ReadingProgress(
    val chapterIndex: Int,
    val fraction: Float,
    val updatedAt: Long,
)

data class ReaderPrefs(
    val fontSize: Int,
    val darkMode: Boolean,
)

object BookRepository {
    private const val STORE_FILE = "books.json"
    private const val PROGRESS_FILE = "progress.json"
    private const val READER_PREFS_FILE = "reader_prefs.json"

    const val DEFAULT_FONT_SIZE = 18
    const val MIN_FONT_SIZE = 14
    const val MAX_FONT_SIZE = 28

    private val _books = mutableStateListOf<Book>()
    val books: List<Book> get() = _books

    private val progressStore: MutableMap<String, ReadingProgress> = mutableMapOf()

    private var readerPrefs: ReaderPrefs = ReaderPrefs(DEFAULT_FONT_SIZE, false)

    @Volatile
    private var initialized = false
    private lateinit var storeFile: File
    private lateinit var progressFile: File
    private lateinit var readerPrefsFile: File

    fun init(context: Context) {
        if (initialized) return
        synchronized(this) {
            if (initialized) return
            storeFile = File(context.filesDir, STORE_FILE)
            progressFile = File(context.filesDir, PROGRESS_FILE)
            readerPrefsFile = File(context.filesDir, READER_PREFS_FILE)
            if (storeFile.exists()) {
                runCatching { load() }.onFailure {
                    _books.clear()
                    persist()
                }
            }
            runCatching { loadProgress() }.onFailure {
                progressStore.clear()
            }
            runCatching { loadReaderPrefs() }.onFailure {
                readerPrefs = ReaderPrefs(DEFAULT_FONT_SIZE, false)
            }
            initialized = true
        }
    }

    fun addImportedBook(book: Book) {
        _books.add(book)
        persist()
    }

    fun findById(id: String): Book? = _books.firstOrNull { it.id == id }

    fun progressOf(bookId: String): ReadingProgress? = progressStore[bookId]

    fun saveProgress(bookId: String, chapterIndex: Int, fraction: Float) {
        val safeFraction = fraction.coerceIn(0f, 1f)
        val safeChapter = chapterIndex.coerceAtLeast(0)
        progressStore[bookId] = ReadingProgress(
            chapterIndex = safeChapter,
            fraction = safeFraction,
            updatedAt = System.currentTimeMillis(),
        )
        persistProgress()
    }

    fun removeProgress(bookId: String) {
        if (progressStore.remove(bookId) != null) {
            persistProgress()
        }
    }

    fun getReaderPrefs(): ReaderPrefs = readerPrefs

    fun saveReaderPrefs(prefs: ReaderPrefs) {
        readerPrefs = prefs.copy(fontSize = prefs.fontSize.coerceIn(MIN_FONT_SIZE, MAX_FONT_SIZE))
        persistReaderPrefs()
    }

    fun updateTocEntries(bookId: String, entries: List<TocEntry>) {
        val idx = _books.indexOfFirst { it.id == bookId }
        if (idx < 0) return
        _books[idx] = _books[idx].copy(tocEntries = entries)
        persist()
    }

    fun removeBook(bookId: String) {
        val idx = _books.indexOfFirst { it.id == bookId }
        if (idx < 0) return
        val removed = _books.removeAt(idx)
        persist()
        removeProgress(bookId)
        val dir = removed.bookDir
        if (!dir.isNullOrBlank()) {
            runCatching { File(dir).deleteRecursively() }
        }
    }

    private fun load() {
        val text = storeFile.readText(Charsets.UTF_8)
        val arr = JSONArray(text)
        val loaded = mutableListOf<Book>()
        var cleanedLegacy = false
        for (i in 0 until arr.length()) {
            val obj = arr.getJSONObject(i)
            val sourceStr = obj.optString("source", BookSource.EPUB.name)
            val source = runCatching { BookSource.valueOf(sourceStr) }.getOrNull() ?: BookSource.EPUB
            if (source == BookSource.BUILTIN) {
                cleanedLegacy = true
                continue
            }
            val id = obj.getString("id")
            val colorArgb = if (obj.has("colorArgb")) obj.getLong("colorArgb") else 0xFF808080L
            val spineArr = obj.optJSONArray("spine")
            val spine = if (spineArr != null) {
                (0 until spineArr.length()).map { spineArr.getString(it) }
            } else {
                emptyList()
            }
            val tocArr = obj.optJSONArray("tocEntries")
            val tocEntries = if (tocArr != null) {
                (0 until tocArr.length()).mapNotNull { i ->
                    val entryObj = tocArr.optJSONObject(i) ?: return@mapNotNull null
                    TocEntry(
                        spineIndex = entryObj.optInt("spineIndex", -1),
                        title = entryObj.optString("title", ""),
                        depth = entryObj.optInt("depth", 0).coerceAtLeast(0),
                    ).takeIf { it.spineIndex >= 0 && it.title.isNotBlank() }
                }
            } else {
                emptyList()
            }
            loaded += Book(
                id = id,
                title = obj.getString("title"),
                author = obj.optString("author", "未知作者"),
                coverColor = Color(colorArgb.toInt()),
                source = source,
                coverPath = obj.optString("coverPath").ifBlank { null },
                bookDir = obj.optString("bookDir").ifBlank { null },
                opfDir = obj.optString("opfDir").ifBlank { null },
                spine = spine,
                tocEntries = tocEntries,
            )
        }
        _books.clear()
        _books.addAll(loaded)
        if (cleanedLegacy) persist()
    }

    private fun persist() {
        val arr = JSONArray()
        for (book in _books) {
            val obj = JSONObject()
            obj.put("id", book.id)
            obj.put("title", book.title)
            obj.put("author", book.author)
            obj.put("source", book.source.name)
            obj.put("colorArgb", colorToArgb(book.coverColor))
            book.coverPath?.let { obj.put("coverPath", it) }
            book.bookDir?.let { obj.put("bookDir", it) }
            book.opfDir?.let { obj.put("opfDir", it) }
            if (book.spine.isNotEmpty()) {
                val spineArr = JSONArray()
                book.spine.forEach { spineArr.put(it) }
                obj.put("spine", spineArr)
            }
            if (book.tocEntries.isNotEmpty()) {
                val tocArr = JSONArray()
                for (entry in book.tocEntries) {
                    val entryObj = JSONObject()
                    entryObj.put("spineIndex", entry.spineIndex)
                    entryObj.put("title", entry.title)
                    entryObj.put("depth", entry.depth)
                    tocArr.put(entryObj)
                }
                obj.put("tocEntries", tocArr)
            }
            arr.put(obj)
        }
        storeFile.writeText(arr.toString(), Charsets.UTF_8)
    }

    private fun loadProgress() {
        if (!progressFile.exists()) {
            progressStore.clear()
            return
        }
        val text = progressFile.readText(Charsets.UTF_8)
        if (text.isBlank()) {
            progressStore.clear()
            return
        }
        val obj = JSONObject(text)
        progressStore.clear()
        val keys = obj.keys()
        while (keys.hasNext()) {
            val bookId = keys.next()
            val item = obj.optJSONObject(bookId) ?: continue
            val chapterIndex = item.optInt("chapterIndex", 0).coerceAtLeast(0)
            val fraction = item.optDouble("fraction", 0.0).toFloat().coerceIn(0f, 1f)
            val updatedAt = item.optLong("updatedAt", 0L)
            progressStore[bookId] = ReadingProgress(chapterIndex, fraction, updatedAt)
        }
    }

    private fun persistProgress() {
        val obj = JSONObject()
        for ((bookId, progress) in progressStore) {
            val item = JSONObject()
            item.put("chapterIndex", progress.chapterIndex)
            item.put("fraction", progress.fraction.toDouble())
            item.put("updatedAt", progress.updatedAt)
            obj.put(bookId, item)
        }
        progressFile.writeText(obj.toString(), Charsets.UTF_8)
    }

    private fun loadReaderPrefs() {
        if (!readerPrefsFile.exists()) {
            readerPrefs = ReaderPrefs(DEFAULT_FONT_SIZE, false)
            return
        }
        val text = readerPrefsFile.readText(Charsets.UTF_8)
        if (text.isBlank()) {
            readerPrefs = ReaderPrefs(DEFAULT_FONT_SIZE, false)
            return
        }
        val obj = JSONObject(text)
        val fontSize = obj.optInt("fontSize", DEFAULT_FONT_SIZE).coerceIn(MIN_FONT_SIZE, MAX_FONT_SIZE)
        val darkMode = obj.optBoolean("darkMode", false)
        readerPrefs = ReaderPrefs(fontSize, darkMode)
    }

    private fun persistReaderPrefs() {
        val obj = JSONObject()
        obj.put("fontSize", readerPrefs.fontSize)
        obj.put("darkMode", readerPrefs.darkMode)
        readerPrefsFile.writeText(obj.toString(), Charsets.UTF_8)
    }

    private fun colorToArgb(color: Color): Long {
        val a = (color.alpha * 255).toInt() and 0xFF
        val r = (color.red * 255).toInt() and 0xFF
        val g = (color.green * 255).toInt() and 0xFF
        val b = (color.blue * 255).toInt() and 0xFF
        return ((a.toLong() shl 24) or (r.toLong() shl 16) or (g.toLong() shl 8) or b.toLong()) and 0xFFFFFFFFL
    }
}
