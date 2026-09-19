package com.example.orange.ui.reader

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.graphics.Rect
import android.util.AttributeSet
import com.example.orange.data.logging.AppLog as Log
import android.view.ActionMode
import android.view.Menu
import android.view.MenuItem
import android.view.View
import android.webkit.WebView
import org.json.JSONObject

class LookupWebView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = 0,
) : WebView(context, attrs, defStyleAttr) {

    var onLookupSelection: ((
        word: String,
        anchorRectInWindow: Rect,
        context: String,
        wordStart: Int,
        wordEnd: Int,
        paragraph: String,
        selectionStart: Int,
        selectionEnd: Int,
    ) -> Unit)? = null

    var onAnalyzeSelection: ((sentence: String) -> Unit)? = null

    var onCopySelection: ((text: String) -> Unit)? = null

    override fun startActionMode(callback: ActionMode.Callback): ActionMode {
        return super.startActionMode(wrap(callback))
    }

    override fun startActionMode(callback: ActionMode.Callback, type: Int): ActionMode {
        return super.startActionMode(wrap(callback), type)
    }

    private fun wrap(inner: ActionMode.Callback): ActionMode.Callback {
        return if (inner is ActionMode.Callback2) WrappedCallback2(inner) else WrappedCallback(inner)
    }

    private open inner class WrappedCallback(private val inner: ActionMode.Callback) : ActionMode.Callback {
        override fun onCreateActionMode(mode: ActionMode, menu: Menu): Boolean {
            val ok = inner.onCreateActionMode(mode, menu)
            trimMenu(menu)
            return ok || true
        }

        override fun onPrepareActionMode(mode: ActionMode, menu: Menu): Boolean {
            val changed = inner.onPrepareActionMode(mode, menu)
            trimMenu(menu)
            return changed || true
        }

        override fun onActionItemClicked(mode: ActionMode, item: MenuItem): Boolean {
            if (item.itemId == MENU_ID_LOOKUP) {
                runSelectionLookup { mode.finish() }
                return true
            }
            if (item.itemId == MENU_ID_ANALYZE) {
                runSelectionAnalyze { mode.finish() }
                return true
            }
            if (item.itemId == MENU_ID_COPY) {
                runSelectionCopy { mode.finish() }
                return true
            }
            return inner.onActionItemClicked(mode, item)
        }

        override fun onDestroyActionMode(mode: ActionMode) {
            inner.onDestroyActionMode(mode)
        }
    }

    private inner class WrappedCallback2(private val innerCallback2: ActionMode.Callback2) : ActionMode.Callback2() {
        override fun onCreateActionMode(mode: ActionMode, menu: Menu): Boolean {
            val ok = innerCallback2.onCreateActionMode(mode, menu)
            trimMenu(menu)
            return ok || true
        }

        override fun onPrepareActionMode(mode: ActionMode, menu: Menu): Boolean {
            val changed = innerCallback2.onPrepareActionMode(mode, menu)
            trimMenu(menu)
            return changed || true
        }

        override fun onActionItemClicked(mode: ActionMode, item: MenuItem): Boolean {
            if (item.itemId == MENU_ID_LOOKUP) {
                runSelectionLookup { mode.finish() }
                return true
            }
            if (item.itemId == MENU_ID_ANALYZE) {
                runSelectionAnalyze { mode.finish() }
                return true
            }
            if (item.itemId == MENU_ID_COPY) {
                runSelectionCopy { mode.finish() }
                return true
            }
            return innerCallback2.onActionItemClicked(mode, item)
        }

        override fun onDestroyActionMode(mode: ActionMode) {
            innerCallback2.onDestroyActionMode(mode)
        }

        override fun onGetContentRect(mode: ActionMode?, view: View?, outRect: Rect?) {
            innerCallback2.onGetContentRect(mode, view, outRect)
        }
    }

    private fun trimMenu(menu: Menu) {
        var i = 0
        while (i < menu.size()) {
            val item = menu.getItem(i)
            if (item.itemId != MENU_ID_LOOKUP && item.itemId != MENU_ID_ANALYZE && item.itemId != MENU_ID_COPY) {
                menu.removeItem(item.itemId)
            } else {
                i++
            }
        }
        if (menu.findItem(MENU_ID_LOOKUP) == null) {
            menu.add(Menu.NONE, MENU_ID_LOOKUP, 1, "查词")
                .setShowAsActionFlags(MenuItem.SHOW_AS_ACTION_ALWAYS)
        }
        if (menu.findItem(MENU_ID_ANALYZE) == null) {
            menu.add(Menu.NONE, MENU_ID_ANALYZE, 2, "分析")
                .setShowAsActionFlags(MenuItem.SHOW_AS_ACTION_ALWAYS)
        }
        if (menu.findItem(MENU_ID_COPY) == null) {
            menu.add(Menu.NONE, MENU_ID_COPY, 3, "复制")
                .setShowAsActionFlags(MenuItem.SHOW_AS_ACTION_ALWAYS)
        }
    }

    private fun runSelectionLookup(afterDispatch: () -> Unit) {
        evaluateJavascript(SELECTION_JS) { raw ->
            val cb = onLookupSelection
            afterDispatch()
            if (cb == null) return@evaluateJavascript
            val payload = parseSelectionPayload(raw) ?: return@evaluateJavascript
            val trimmed = payload.text.trim()
            if (trimmed.isEmpty()) return@evaluateJavascript
            val hasInnerWhitespace = trimmed.any { it.isWhitespace() }
            val normalized = if (hasInnerWhitespace) {
                trimmed.split(WHITESPACE_REGEX).joinToString(" ")
            } else {
                trimmed
            }
            if (normalized.length > MAX_PHRASE_LEN) return@evaluateJavascript
            if (!hasInnerWhitespace) {
                if (!WORD_REGEX.matches(normalized)) return@evaluateJavascript
            } else {
                if (!PHRASE_REGEX.matches(normalized)) return@evaluateJavascript
            }

            val density = resources.displayMetrics.density
            val loc = IntArray(2)
            getLocationInWindow(loc)
            val left = (payload.left * density).toInt() + loc[0]
            val top = (payload.top * density).toInt() + loc[1]
            val right = (payload.right * density).toInt() + loc[0]
            val bottom = (payload.bottom * density).toInt() + loc[1]
            cb(
                normalized,
                Rect(left, top, right, bottom),
                payload.context,
                payload.wordStart,
                payload.wordEnd,
                payload.paragraph,
                payload.selectionStart,
                payload.selectionEnd,
            )
        }
    }

    private fun runSelectionAnalyze(afterDispatch: () -> Unit) {
        Log.d(TAG, "runSelectionAnalyze: click received")
        evaluateJavascript(SELECTION_JS) { raw ->
            val cb = onAnalyzeSelection
            afterDispatch()
            if (cb == null) {
                Log.w(TAG, "runSelectionAnalyze: callback is null, drop")
                return@evaluateJavascript
            }
            val payload = parseSelectionPayload(raw)
            if (payload == null) {
                Log.w(TAG, "runSelectionAnalyze: parse payload failed raw=${raw?.take(200)}")
                return@evaluateJavascript
            }
            val trimmed = payload.text.trim()
            if (trimmed.isEmpty()) {
                Log.w(TAG, "runSelectionAnalyze: trimmed empty, drop")
                return@evaluateJavascript
            }
            if (!trimmed.any { it.isWhitespace() }) {
                Log.d(TAG, "runSelectionAnalyze: single word, drop text=${trimmed.take(80)}")
                return@evaluateJavascript
            }
            val normalized = trimmed.split(WHITESPACE_REGEX).joinToString(" ").normalizeSentence()
            if (normalized.length > MAX_SENTENCE_LEN) {
                Log.w(TAG, "runSelectionAnalyze: too long len=${normalized.length}")
                return@evaluateJavascript
            }
            // 不再做字符集正则校验：后端 normalizeSentence 会归一化 smart quote 等，
            // 前端过严校验会拦掉 EPUB 里 “”, ’, —, … 等常见字符。
            Log.d(TAG, "runSelectionAnalyze: dispatch sentence=${normalized.take(120)} len=${normalized.length}")
            cb(normalized)
        }
    }

    private fun runSelectionCopy(afterDispatch: () -> Unit) {
        evaluateJavascript(SELECTION_JS) { raw ->
            afterDispatch()
            val payload = parseSelectionPayload(raw)
            val text = payload?.text?.trim().orEmpty()
            if (text.isEmpty()) {
                Log.w(TAG, "runSelectionCopy: empty selection, skip")
                return@evaluateJavascript
            }
            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
            if (clipboard == null) {
                Log.w(TAG, "runSelectionCopy: clipboard service unavailable")
                return@evaluateJavascript
            }
            clipboard.setPrimaryClip(ClipData.newPlainText("reader-selection", text))
            onCopySelection?.invoke(text)
        }
    }

    private fun parseSelectionPayload(raw: String?): SelectionPayload? {
        if (raw.isNullOrEmpty() || raw == "null") return null
        val jsonString = if (raw.startsWith("\"") && raw.endsWith("\"")) {
            JSONObject("{\"v\":$raw}").optString("v", "")
        } else {
            raw
        }
        if (jsonString.isEmpty() || jsonString == "null") return null
        return runCatching {
            val obj = JSONObject(jsonString)
            SelectionPayload(
                text = obj.optString("text", ""),
                top = obj.optDouble("top", 0.0),
                left = obj.optDouble("left", 0.0),
                right = obj.optDouble("right", 0.0),
                bottom = obj.optDouble("bottom", 0.0),
                context = obj.optString("context", ""),
                wordStart = obj.optInt("wordStart", -1),
                wordEnd = obj.optInt("wordEnd", -1),
                paragraph = obj.optString("paragraph", ""),
                selectionStart = obj.optInt("selectionStart", -1),
                selectionEnd = obj.optInt("selectionEnd", -1),
            )
        }.getOrNull()
    }

    private data class SelectionPayload(
        val text: String,
        val top: Double,
        val left: Double,
        val right: Double,
        val bottom: Double,
        val context: String,
        val wordStart: Int,
        val wordEnd: Int,
        val paragraph: String,
        val selectionStart: Int,
        val selectionEnd: Int,
    )

    companion object {
        private const val TAG = "LookupWebView"
        const val MENU_ID_LOOKUP = 0x0100_0100
        const val MENU_ID_ANALYZE = 0x0100_0101
        const val MENU_ID_COPY = 0x0100_0102

        private val WORD_REGEX = Regex("^[A-Za-z][A-Za-z'-]*$")
        private val PHRASE_REGEX = Regex("^[A-Za-z][A-Za-z'-]*(?: [A-Za-z][A-Za-z'-]*){1,7}$")
        private val WHITESPACE_REGEX = Regex("\\s+")
        private const val MAX_PHRASE_LEN = 80
        private const val MAX_SENTENCE_LEN = 500
        private const val SELECTION_JS = """
            (function(){
              try {
                var s = window.getSelection();
                if (!s || s.rangeCount === 0) return null;
                var range = s.getRangeAt(0);
                var r = range.getBoundingClientRect();
                var text = s.toString();
                var container = null;
                var startNode = range.startContainer;
                var el = (startNode && startNode.nodeType === 1) ? startNode : (startNode ? startNode.parentElement : null);
                if (el && el.closest) {
                  container = el.closest('p, li, blockquote');
                  if (!container) container = el.closest('div, article, section');
                }
                if (!container) container = document.body;
                var pre = document.createRange();
                pre.selectNodeContents(container);
                pre.setEnd(range.startContainer, range.startOffset);
                var wordStart = pre.toString().length;
                var wordEnd = wordStart + text.length;
                var context = container.innerText || container.textContent || '';
                var selectedLeading = text.length - text.replace(/^\s+/, '').length;
                var selectedTrailing = text.length - text.replace(/\s+$/, '').length;
                var selectedStart = wordStart + selectedLeading;
                var selectedEnd = wordEnd - selectedTrailing;
                var paragraph = context;
                var LIMIT = 1000;
                if (context.length > LIMIT) {
                  var half = 400;
                  var s0 = Math.max(0, wordStart - half);
                  var e0 = Math.min(context.length, wordEnd + half);
                  context = context.slice(s0, e0);
                  wordStart -= s0;
                  wordEnd -= s0;
                }
                return JSON.stringify({
                  text: text,
                  top: r.top, left: r.left, right: r.right, bottom: r.bottom,
                  context: context, wordStart: wordStart, wordEnd: wordEnd,
                  paragraph: paragraph,
                  selectionStart: selectedStart,
                  selectionEnd: selectedEnd
                });
              } catch (e) { return null; }
            })();
        """
    }
}

private fun String.normalizeSentence(): String {
    return this
        .replace('\u2018', '\'')
        .replace('\u2019', '\'')
        .replace('\u201C', '"')
        .replace('\u201D', '"')
        .replace('\u2013', '-')
        .replace('\u2014', '-')
        .replace("\u2026", "...")
        .replace('\u00A0', ' ')
}
