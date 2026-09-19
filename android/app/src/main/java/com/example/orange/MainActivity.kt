package com.example.orange

import android.Manifest
import android.os.Bundle
import android.os.Build
import android.content.pm.PackageManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.core.content.ContextCompat
import com.example.orange.data.BookRepository
import com.example.orange.data.audiosync.AudioSyncScheduler
import com.example.orange.data.contextsync.ContextSyncPreferences
import com.example.orange.data.contextsync.ContextVideoSyncScheduler
import com.example.orange.data.learning.LearningApi
import com.example.orange.data.word.ContextVideo
import com.example.orange.data.word.WordServiceConfig
import com.example.orange.ui.bookshelf.BookshelfScreen
import com.example.orange.ui.context.ContextVideoScreen
import com.example.orange.ui.learning.LearningPlanScreen
import com.example.orange.ui.learning.LearningSettingsScreen
import com.example.orange.ui.learning.WordLearningScreen
import com.example.orange.ui.reader.ReaderScreen
import com.example.orange.ui.sentence.SentenceDetailScreen
import com.example.orange.ui.sentence.SentenceListScreen
import com.example.orange.ui.theme.OrangeTheme
import com.example.orange.ui.wordlist.PhraseDetailScreen
import com.example.orange.ui.wordlist.WordDetailScreen
import com.example.orange.ui.wordlist.WordListScreen

class MainActivity : ComponentActivity() {
    private val localNetworkPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        if (granted) {
            ContextVideoSyncScheduler.start(applicationContext)
            AudioSyncScheduler.start(applicationContext)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        BookRepository.init(applicationContext)
        WordServiceConfig.init(applicationContext)
        LearningApi.init(applicationContext)
        requestLocalNetworkPermissionOnce()
        enableEdgeToEdge()
        setContent {
            OrangeTheme {
                OrangeApp()
            }
        }
    }

    private fun requestLocalNetworkPermissionOnce() {
        if (Build.VERSION.SDK_INT < 37) {
            ContextVideoSyncScheduler.start(applicationContext)
            AudioSyncScheduler.start(applicationContext)
            return
        }
        if (
            ContextCompat.checkSelfPermission(
                this,
                Manifest.permission.ACCESS_LOCAL_NETWORK,
            ) == PackageManager.PERMISSION_GRANTED
        ) {
            ContextVideoSyncScheduler.start(applicationContext)
            AudioSyncScheduler.start(applicationContext)
            return
        }
        val preferences = ContextSyncPreferences(this)
        if (preferences.permissionAsked) return
        preferences.permissionAsked = true
        localNetworkPermissionLauncher.launch(Manifest.permission.ACCESS_LOCAL_NETWORK)
    }
}

private enum class AppTab { WORDS, LEARNING, SENTENCES, BOOKSHELF }

@Composable
private fun OrangeApp() {
    var currentBookId by rememberSaveable { mutableStateOf<String?>(null) }
    var selectedItemType by rememberSaveable { mutableStateOf<String?>(null) }
    var selectedItemId by rememberSaveable { mutableStateOf(0L) }
    var selectedItemText by rememberSaveable { mutableStateOf("") }
    var learningSessionId by rememberSaveable { mutableStateOf<Long?>(null) }
    var showLearningSettings by rememberSaveable { mutableStateOf(false) }
    var selectedTab by rememberSaveable { mutableStateOf(AppTab.WORDS) }
    var selectedSentenceId by rememberSaveable { mutableStateOf<Long?>(null) }
    var selectedContextVideo by remember { mutableStateOf<ContextVideo?>(null) }
    val books = BookRepository.books
    val currentBook = currentBookId?.let { id -> books.firstOrNull { it.id == id } }

    Box(Modifier.fillMaxSize()) {
        when {
            showLearningSettings -> {
                BackHandler { showLearningSettings = false }
                LearningSettingsScreen(
                    onBack = { showLearningSettings = false },
                    modifier = Modifier.fillMaxSize(),
                )
            }
            learningSessionId != null -> {
                BackHandler { learningSessionId = null }
                WordLearningScreen(
                    sessionId = learningSessionId!!,
                    onBack = { learningSessionId = null },
                    onPlayContextVideo = { selectedContextVideo = it },
                    modifier = Modifier.fillMaxSize(),
                )
            }
            currentBook != null -> {
                ReaderScreen(
                    book = currentBook,
                    onBack = { currentBookId = null },
                    modifier = Modifier.fillMaxSize(),
                )
            }
            selectedSentenceId != null -> {
                BackHandler { selectedSentenceId = null }
                SentenceDetailScreen(
                    sentenceId = selectedSentenceId!!,
                    onBack = { selectedSentenceId = null },
                    modifier = Modifier.fillMaxSize(),
                )
            }
            selectedItemType != null -> {
                val clearSelection = {
                    selectedItemType = null
                    selectedItemId = 0L
                    selectedItemText = ""
                }
                BackHandler(onBack = clearSelection)
                if (selectedItemType == "phrase") {
                    PhraseDetailScreen(
                        itemId = selectedItemId,
                        fallbackText = selectedItemText,
                        onBack = clearSelection,
                        onPlayContextVideo = { selectedContextVideo = it },
                        modifier = Modifier.fillMaxSize(),
                    )
                } else {
                    WordDetailScreen(
                        itemId = selectedItemId,
                        word = selectedItemText,
                        onBack = clearSelection,
                        onPlayContextVideo = { selectedContextVideo = it },
                        modifier = Modifier.fillMaxSize(),
                    )
                }
            }
            else -> {
                Scaffold(
                    modifier = Modifier.fillMaxSize(),
                    bottomBar = {
                        NavigationBar {
                            NavigationBarItem(
                                selected = selectedTab == AppTab.WORDS,
                                onClick = { selectedTab = AppTab.WORDS },
                                icon = {
                                    Icon(
                                        painter = painterResource(id = R.drawable.ic_word),
                                        contentDescription = "单词",
                                    )
                                },
                                label = { Text("单词") },
                            )
                            NavigationBarItem(
                                selected = selectedTab == AppTab.LEARNING,
                                onClick = { selectedTab = AppTab.LEARNING },
                                icon = {
                                    Icon(
                                        painter = painterResource(id = R.drawable.ic_learning),
                                        contentDescription = "学习",
                                    )
                                },
                                label = { Text("学习") },
                            )
                            NavigationBarItem(
                                selected = selectedTab == AppTab.SENTENCES,
                                onClick = { selectedTab = AppTab.SENTENCES },
                                icon = {
                                    Icon(
                                        painter = painterResource(id = R.drawable.ic_sentence),
                                        contentDescription = "句子",
                                    )
                                },
                                label = { Text("句子") },
                            )
                            NavigationBarItem(
                                selected = selectedTab == AppTab.BOOKSHELF,
                                onClick = { selectedTab = AppTab.BOOKSHELF },
                                icon = {
                                    Icon(
                                        painter = painterResource(id = R.drawable.ic_bookshelf),
                                        contentDescription = "书架",
                                    )
                                },
                                label = { Text("书架") },
                            )
                        }
                    }
                ) { innerPadding ->
                    when (selectedTab) {
                        AppTab.WORDS -> {
                            WordListScreen(
                                onItemClick = { item ->
                                    if (item.clientId == null && item.itemId > 0L) {
                                        selectedItemType = item.itemType
                                        selectedItemId = item.itemId
                                        selectedItemText = item.text
                                    }
                                },
                                onWordSearch = { word ->
                                    selectedItemType = "word"
                                    selectedItemId = 0L
                                    selectedItemText = word
                                },
                                modifier = Modifier.fillMaxSize(),
                                bottomPadding = innerPadding.calculateBottomPadding(),
                            )
                        }
                        AppTab.LEARNING -> {
                            LearningPlanScreen(
                                onStartLearning = { sessionId -> learningSessionId = sessionId },
                                onOpenSettings = { showLearningSettings = true },
                                bottomPadding = innerPadding.calculateBottomPadding(),
                                modifier = Modifier.fillMaxSize(),
                            )
                        }
                        AppTab.SENTENCES -> {
                            SentenceListScreen(
                                onSentenceClick = { selectedSentenceId = it },
                                bottomPadding = innerPadding.calculateBottomPadding(),
                                modifier = Modifier.fillMaxSize(),
                            )
                        }
                        AppTab.BOOKSHELF -> {
                            BookshelfScreen(
                                books = books,
                                onBookClick = { book -> currentBookId = book.id },
                                bottomPadding = innerPadding.calculateBottomPadding(),
                                modifier = Modifier.fillMaxSize(),
                            )
                        }
                    }
                }
            }
        }
        selectedContextVideo?.let { video ->
            ContextVideoScreen(
                video = video,
                onBack = { selectedContextVideo = null },
                modifier = Modifier.fillMaxSize(),
            )
        }
    }
}
