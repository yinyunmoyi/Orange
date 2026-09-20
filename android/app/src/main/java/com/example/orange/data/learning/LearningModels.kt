package com.example.orange.data.learning

import com.example.orange.data.word.ContextSentence

const val LEARNING_ITEM_WORD = 1
const val LEARNING_ITEM_PHRASE = 2

data class LearningStatusCounts(
    val notStarted: Int,
    val learning: Int,
    val learned: Int,
    val mastered: Int,
)

data class LearningPlan(
    val sessionId: Long,
    val sessionStatus: String,
    val todayTotal: Int,
    val todayNew: Int,
    val todayReview: Int,
    val todayRetry: Int,
    val totalItems: Int,
    val statusCounts: LearningStatusCounts,
    val hasCurrentCard: Boolean,
    val canExtend: Boolean = false,
)

data class LearningSettings(
    val dailyNewLimit: Int,
    val dailyReviewLimit: Int,
    val remainingNewCount: Int,
    val studyDate: String,
)

data class LearningMeaning(val pos: String, val text: String)

data class LearningCard(
    val itemType: Int,
    val itemId: Long,
    val word: String,
    val level: Int = 0,
    val phonetic: String,
    val audioUrl: String,
    val chinese: List<LearningMeaning>,
    val contexts: List<ContextSentence>,
    val english: List<LearningMeaning>,
    val note: String,
    val pos: String = "",
    val collins: Int = 0,
    val oxford: Int = 0,
    val tags: List<String> = emptyList(),
)

data class LearningSessionProgress(
    val total: Int,
    val notStarted: Int,
    val inProgress: Int,
    val completed: Int,
)

data class LearningCurrent(
    val sessionId: Long,
    val turnNo: Int,
    val queueItemId: Long,
    val queueType: String,
    val remaining: Int,
    val completed: Boolean,
    val progress: LearningSessionProgress,
    val card: LearningCard?,
)
