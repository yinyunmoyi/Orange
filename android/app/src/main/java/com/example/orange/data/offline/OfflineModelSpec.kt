package com.example.orange.data.offline

object OfflineModelSpec {
    const val FILE_NAME = "Qwen3-1.7B_dynamic_wi4b32_afp32.litertlm"
    const val FILE_SIZE_BYTES = 977_184_032L
    const val SHA256 = "2eeffef7b51bc3e1225ea69fe7aa5f417397934b56a5b6c20cc068d6fd2c918b"
    const val MIN_MEMORY_BYTES = 6L * 1024 * 1024 * 1024
    const val MIN_FREE_SPACE_BYTES = 1_500_000_000L
    const val MAX_VALIDATION_TOKENS = 32
    const val VALIDATION_TIMEOUT_MILLIS = 120_000L

    const val SYSTEM_PROMPT = "You are a concise bilingual assistant."
    const val USER_PROMPT =
        "Reply with one short Chinese sentence confirming that local inference is working."
}

enum class OfflineBackend {
    GPU,
    CPU,
}

data class OfflineModelFingerprint(
    val path: String,
    val length: Long,
    val lastModified: Long,
    val sha256: String,
)

data class OfflineCapabilityResult(
    val fingerprint: OfflineModelFingerprint,
    val appVersionCode: Long,
    val backend: OfflineBackend,
    val reply: String,
    val initializationMillis: Long,
    val firstTokenMillis: Long,
    val totalMillis: Long,
)
