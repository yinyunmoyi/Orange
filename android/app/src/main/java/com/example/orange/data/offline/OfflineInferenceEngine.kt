package com.example.orange.data.offline

import kotlinx.coroutines.flow.Flow

interface OfflineInferenceEngine {
    suspend fun initialize(modelPath: String): Result<OfflineBackend>

    fun generate(
        systemPrompt: String,
        userPrompt: String,
        maxTokens: Int,
    ): Flow<String>

    suspend fun close()
}

fun interface OfflineInferenceEngineFactory {
    fun create(): OfflineInferenceEngine
}
