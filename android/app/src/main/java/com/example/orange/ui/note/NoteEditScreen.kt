package com.example.orange.ui.note

import com.example.orange.data.logging.AppLog as Log
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp
import com.example.orange.R
import com.example.orange.data.standalone.StandaloneItemType
import com.example.orange.data.standalone.StandaloneRepository
import com.example.orange.data.word.WordApi
import kotlinx.coroutines.launch

const val MAX_NOTE_CODE_POINTS = 2000

internal fun noteCodePointCount(text: String): Int = text.codePointCount(0, text.length)

internal fun canSaveNote(text: String, saving: Boolean): Boolean =
    !saving && noteCodePointCount(text) <= MAX_NOTE_CODE_POINTS

@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun NoteEditScreen(
    itemType: String,
    text: String,
    initialNote: String,
    onBack: () -> Unit,
    onSaved: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var draft by rememberSaveable(itemType, text) { mutableStateOf(initialNote) }
    var saving by remember { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val focusRequester = remember { FocusRequester() }
    val count = noteCodePointCount(draft)
    val overLimit = count > MAX_NOTE_CODE_POINTS

    BackHandler(enabled = !saving, onBack = onBack)
    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }

    Scaffold(
        modifier = modifier.fillMaxSize(),
        topBar = {
            TopAppBar(
                title = { Text(text) },
                navigationIcon = {
                    IconButton(onClick = onBack, enabled = !saving) {
                        Icon(
                            painter = painterResource(R.drawable.ic_back),
                            contentDescription = "返回",
                        )
                    }
                },
                actions = {
                    TextButton(
                        enabled = canSaveNote(draft, saving),
                        onClick = {
                            saving = true
                            scope.launch {
                                val saveResult = if (StandaloneRepository.isEnabled()) {
                                    runCatching {
                                        StandaloneRepository.saveNote(
                                            StandaloneItemType.fromWire(itemType),
                                            text,
                                            draft,
                                        )
                                        draft
                                    }
                                } else {
                                    WordApi.saveNote(itemType, text, draft).map { it.note }
                                }
                                saveResult.fold(
                                    onSuccess = {
                                        saving = false
                                        onSaved(it)
                                    },
                                    onFailure = { error ->
                                        Log.e(
                                            "NoteEdit",
                                            "save failed itemType=$itemType text=$text noteLength=$count",
                                            error,
                                        )
                                        saving = false
                                        snackbarHostState.showSnackbar("保存失败，请重试")
                                    },
                                )
                            }
                        },
                    ) {
                        Text(if (saving) "保存中" else "完成")
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp),
        ) {
            OutlinedTextField(
                value = draft,
                onValueChange = { draft = it },
                label = { Text("备注") },
                placeholder = { Text("记录关于这个${if (itemType == "phrase") "短语" else "单词"}的内容") },
                supportingText = {
                    Text(
                        text = "$count/$MAX_NOTE_CODE_POINTS",
                        color = if (overLimit) {
                            MaterialTheme.colorScheme.error
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        },
                    )
                },
                isError = overLimit,
                enabled = !saving,
                modifier = Modifier
                    .fillMaxWidth()
                    .fillMaxHeight(1f / 3f)
                    .focusRequester(focusRequester),
            )
        }
    }
}
