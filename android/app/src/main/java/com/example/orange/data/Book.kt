package com.example.orange.data

import androidx.compose.ui.graphics.Color

enum class BookSource {
    BUILTIN,
    EPUB,
}

data class TocEntry(
    val spineIndex: Int,
    val title: String,
    val depth: Int,
)

data class Book(
    val id: String,
    val title: String,
    val author: String,
    val coverColor: Color,
    val source: BookSource = BookSource.BUILTIN,
    val coverPath: String? = null,
    val bookDir: String? = null,
    val opfDir: String? = null,
    val spine: List<String> = emptyList(),
    val tocEntries: List<TocEntry> = emptyList(),
)
