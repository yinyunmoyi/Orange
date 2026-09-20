package com.example.orange.data.learning

data class LearningReviewForecastItem(
    val date: String,
    val count: Int,
)

data class LearningReviewForecast(
    val startDate: String,
    val endDate: String,
    val days: Int,
    val total: Int,
    val items: List<LearningReviewForecastItem>,
)
