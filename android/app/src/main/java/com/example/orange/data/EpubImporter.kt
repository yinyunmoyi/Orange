package com.example.orange.data

import android.content.Context
import android.net.Uri
import android.util.Xml
import androidx.compose.ui.graphics.Color
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.xmlpull.v1.XmlPullParser
import java.io.File
import java.io.IOException
import java.util.UUID
import java.util.zip.ZipInputStream

object EpubImporter {

    suspend fun importEpub(context: Context, uri: Uri): Book = withContext(Dispatchers.IO) {
        val bookId = UUID.randomUUID().toString()
        val booksRoot = File(context.filesDir, "books").apply { mkdirs() }
        val bookDir = File(booksRoot, bookId).apply { mkdirs() }
        try {
            extractZip(context, uri, bookDir)
            val opfRelPath = readContainerXml(bookDir)
                ?: throw IOException("EPUB 缺少 META-INF/container.xml 或 rootfile")
            val opfFile = File(bookDir, opfRelPath)
            if (!opfFile.exists()) throw IOException("OPF 文件不存在: $opfRelPath")
            val opfDirRel = opfFile.parentFile
                ?.relativeToOrNull(bookDir)
                ?.path
                ?: ""
            val parsed = parseOpf(opfFile)
            if (parsed.spine.isEmpty()) throw IOException("EPUB spine 为空，无可读章节")

            val coverPath = parsed.coverHref?.let { href ->
                File(opfFile.parentFile, href).takeIf { it.exists() }?.absolutePath
            }

            val tocEntries = parsed.ncxHref
                ?.let { ncxHref ->
                    val ncxFile = File(opfFile.parentFile, ncxHref)
                    if (ncxFile.exists()) parseNcx(ncxFile, parsed.spine) else emptyList()
                }
                .orEmpty()

            Book(
                id = bookId,
                title = parsed.title.ifBlank { "未命名书籍" },
                author = parsed.author.ifBlank { "未知作者" },
                coverColor = generateColor(parsed.title),
                source = BookSource.EPUB,
                coverPath = coverPath,
                bookDir = bookDir.absolutePath,
                opfDir = opfDirRel,
                spine = parsed.spine,
                tocEntries = tocEntries,
            )
        } catch (t: Throwable) {
            bookDir.deleteRecursively()
            throw t
        }
    }

    fun rebuildTocEntries(book: Book): List<TocEntry> {
        if (book.source != BookSource.EPUB) return emptyList()
        val bookDir = book.bookDir?.let(::File) ?: return emptyList()
        if (!bookDir.exists()) return emptyList()
        val opfRelPath = runCatching { readContainerXml(bookDir) }.getOrNull() ?: return emptyList()
        val opfFile = File(bookDir, opfRelPath)
        if (!opfFile.exists()) return emptyList()
        val parsed = runCatching { parseOpf(opfFile) }.getOrNull() ?: return emptyList()
        val ncxHref = parsed.ncxHref ?: return emptyList()
        val ncxFile = File(opfFile.parentFile, ncxHref)
        if (!ncxFile.exists()) return emptyList()
        return runCatching { parseNcx(ncxFile, parsed.spine) }.getOrDefault(emptyList())
    }

    private fun extractZip(context: Context, uri: Uri, targetRoot: File) {
        val input = context.contentResolver.openInputStream(uri)
            ?: throw IOException("无法打开所选文件")
        input.use { raw ->
            ZipInputStream(raw).use { zip ->
                while (true) {
                    val entry = zip.nextEntry ?: break
                    val name = entry.name
                    if (name.contains("..")) {
                        zip.closeEntry()
                        continue
                    }
                    val outFile = File(targetRoot, name)
                    if (entry.isDirectory) {
                        outFile.mkdirs()
                    } else {
                        outFile.parentFile?.mkdirs()
                        outFile.outputStream().use { out -> zip.copyTo(out) }
                    }
                    zip.closeEntry()
                }
            }
        }
    }

    private fun readContainerXml(bookDir: File): String? {
        val containerFile = File(bookDir, "META-INF/container.xml")
        if (!containerFile.exists()) return null
        containerFile.inputStream().use { stream ->
            val parser = Xml.newPullParser()
            parser.setFeature(XmlPullParser.FEATURE_PROCESS_NAMESPACES, true)
            parser.setInput(stream, null)
            var event = parser.eventType
            while (event != XmlPullParser.END_DOCUMENT) {
                if (event == XmlPullParser.START_TAG && parser.name == "rootfile") {
                    val fullPath = parser.getAttributeValue(null, "full-path")
                    if (!fullPath.isNullOrBlank()) return fullPath
                }
                event = parser.next()
            }
        }
        return null
    }

    private data class ManifestItem(
        val id: String,
        val href: String,
        val mediaType: String?,
        val properties: String?,
    )

    private data class OpfData(
        val title: String,
        val author: String,
        val spine: List<String>,
        val coverHref: String?,
        val ncxHref: String?,
    )

    private fun parseOpf(opfFile: File): OpfData {
        var title = ""
        var author = ""
        val manifest = mutableMapOf<String, ManifestItem>()
        val spineIds = mutableListOf<String>()
        var metaCoverId: String? = null
        var spineTocId: String? = null

        opfFile.inputStream().use { stream ->
            val parser = Xml.newPullParser()
            parser.setFeature(XmlPullParser.FEATURE_PROCESS_NAMESPACES, true)
            parser.setInput(stream, null)

            var inMetadata = false
            var currentMetaTag: String? = null
            val textBuffer = StringBuilder()

            var event = parser.eventType
            while (event != XmlPullParser.END_DOCUMENT) {
                when (event) {
                    XmlPullParser.START_TAG -> {
                        val local = parser.name
                        when (local) {
                            "metadata" -> inMetadata = true
                            "manifest" -> inMetadata = false
                            "spine" -> {
                                inMetadata = false
                                spineTocId = parser.getAttributeValue(null, "toc")
                            }
                            "item" -> {
                                val id = parser.getAttributeValue(null, "id") ?: ""
                                val href = parser.getAttributeValue(null, "href") ?: ""
                                if (id.isNotBlank() && href.isNotBlank()) {
                                    manifest[id] = ManifestItem(
                                        id = id,
                                        href = href,
                                        mediaType = parser.getAttributeValue(null, "media-type"),
                                        properties = parser.getAttributeValue(null, "properties"),
                                    )
                                }
                            }
                            "itemref" -> {
                                val idref = parser.getAttributeValue(null, "idref")
                                if (!idref.isNullOrBlank()) spineIds += idref
                            }
                            "meta" -> {
                                if (inMetadata) {
                                    val nameAttr = parser.getAttributeValue(null, "name")
                                    val contentAttr = parser.getAttributeValue(null, "content")
                                    if (nameAttr == "cover" && !contentAttr.isNullOrBlank()) {
                                        metaCoverId = contentAttr
                                    }
                                }
                            }
                            else -> {
                                if (inMetadata) {
                                    currentMetaTag = local
                                    textBuffer.setLength(0)
                                }
                            }
                        }
                    }
                    XmlPullParser.TEXT -> {
                        if (inMetadata && currentMetaTag != null) {
                            textBuffer.append(parser.text)
                        }
                    }
                    XmlPullParser.END_TAG -> {
                        val local = parser.name
                        if (local == "metadata") {
                            inMetadata = false
                        } else if (inMetadata && local == currentMetaTag) {
                            val value = textBuffer.toString().trim()
                            when (local) {
                                "title" -> if (title.isBlank()) title = value
                                "creator" -> if (author.isBlank()) author = value
                            }
                            currentMetaTag = null
                            textBuffer.setLength(0)
                        }
                    }
                }
                event = parser.next()
            }
        }

        val spineHrefs = spineIds.mapNotNull { id -> manifest[id]?.href }

        val coverItem = manifest.values.firstOrNull { it.properties?.contains("cover-image") == true }
            ?: metaCoverId?.let { manifest[it] }
        val coverHref = coverItem?.href

        val ncxItem = spineTocId?.let { manifest[it] }
            ?: manifest.values.firstOrNull { it.mediaType == "application/x-dtbncx+xml" }
        val ncxHref = ncxItem?.href

        return OpfData(
            title = title,
            author = author,
            spine = spineHrefs,
            coverHref = coverHref,
            ncxHref = ncxHref,
        )
    }

    private data class NavPointRaw(val href: String, val title: String, val depth: Int)

    private fun parseNcx(ncxFile: File, spineHrefs: List<String>): List<TocEntry> {
        val flat = mutableListOf<NavPointRaw>()

        ncxFile.inputStream().use { stream ->
            val parser = Xml.newPullParser()
            parser.setFeature(XmlPullParser.FEATURE_PROCESS_NAMESPACES, true)
            parser.setInput(stream, null)

            var depth = -1
            var inNavMap = false
            var currentHref: String? = null
            val titleStack = ArrayDeque<StringBuilder>()
            var inLabelText = false

            var event = parser.eventType
            while (event != XmlPullParser.END_DOCUMENT) {
                when (event) {
                    XmlPullParser.START_TAG -> {
                        when (parser.name) {
                            "navMap" -> inNavMap = true
                            "navPoint" -> if (inNavMap) {
                                depth += 1
                                titleStack.addLast(StringBuilder())
                                currentHref = null
                            }
                            "text" -> if (inNavMap && titleStack.isNotEmpty()) {
                                inLabelText = true
                            }
                            "content" -> if (inNavMap && titleStack.isNotEmpty()) {
                                val src = parser.getAttributeValue(null, "src")
                                if (!src.isNullOrBlank() && currentHref == null) {
                                    currentHref = src
                                }
                            }
                        }
                    }
                    XmlPullParser.TEXT -> {
                        if (inLabelText && titleStack.isNotEmpty()) {
                            titleStack.last().append(parser.text)
                        }
                    }
                    XmlPullParser.END_TAG -> {
                        when (parser.name) {
                            "navMap" -> inNavMap = false
                            "text" -> inLabelText = false
                            "navPoint" -> if (inNavMap && titleStack.isNotEmpty()) {
                                val title = titleStack.removeLast().toString()
                                    .replace(Regex("\\s+"), " ")
                                    .trim()
                                val href = currentHref
                                if (!href.isNullOrBlank() && title.isNotEmpty()) {
                                    flat += NavPointRaw(href, title, depth)
                                }
                                depth -= 1
                                currentHref = null
                            }
                        }
                    }
                }
                event = parser.next()
            }
        }

        if (flat.isEmpty() || spineHrefs.isEmpty()) return emptyList()

        val seen = HashMap<Int, TocEntry>()
        for (np in flat) {
            val bare = np.href.substringBefore('#')
            val spineIndex = spineHrefs.indexOfFirst { it == bare || it.substringBefore('#') == bare }
            if (spineIndex < 0) continue
            val existing = seen[spineIndex]
            if (existing == null || np.depth < existing.depth) {
                seen[spineIndex] = TocEntry(spineIndex, np.title, np.depth)
            }
        }
        return seen.values.sortedBy { it.spineIndex }
    }

    private fun generateColor(seed: String): Color {
        val hash = seed.hashCode()
        val palette = longArrayOf(
            0xFFEF6C57L, 0xFF6B8ECB, 0xFF2E3B4E, 0xFFF7C948,
            0xFF8E5A3C, 0xFF4A7C59, 0xFF9FB4C7, 0xFFB86BAC,
            0xFF5B7C99, 0xFFD9822B, 0xFF7B5EA7, 0xFF3E8A6E,
        )
        val idx = ((hash % palette.size) + palette.size) % palette.size
        return Color(palette[idx].toInt())
    }
}
