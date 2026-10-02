package com.example.orange.data.mode

enum class AppMode {
    NORMAL,
    STANDALONE,
    OFFLINE;

    val usesLocalData: Boolean
        get() = this != NORMAL
}
