package com.example.orange.data.offline

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LiteRtOfflineInferenceEngineTest {
    @Test
    fun initialize_usesGpuWhenAvailable() = runBlocking {
        val gpu = FakeHandle()
        val created = mutableListOf<OfflineBackend>()
        val engine = LiteRtOfflineInferenceEngine { _, backend ->
            created += backend
            gpu
        }

        val result = engine.initialize("/model")

        assertEquals(OfflineBackend.GPU, result.getOrThrow())
        assertEquals(listOf(OfflineBackend.GPU), created)
        assertFalse(gpu.closed)
        engine.close()
        assertTrue(gpu.closed)
    }

    @Test
    fun initialize_closesFailedGpuAndFallsBackToCpu() = runBlocking {
        val gpu = FakeHandle(initializeError = IllegalStateException("gpu failed"))
        val cpu = FakeHandle()
        val engine = LiteRtOfflineInferenceEngine { _, backend ->
            if (backend == OfflineBackend.GPU) gpu else cpu
        }

        val result = engine.initialize("/model")

        assertEquals(OfflineBackend.CPU, result.getOrThrow())
        assertTrue(gpu.closed)
        assertFalse(cpu.closed)
        engine.close()
        assertTrue(cpu.closed)
    }

    @Test
    fun initialize_reportsBothBackendFailuresAndReleasesResources() = runBlocking {
        val gpu = FakeHandle(initializeError = IllegalStateException("gpu failed"))
        val cpu = FakeHandle(initializeError = IllegalStateException("cpu failed"))
        val engine = LiteRtOfflineInferenceEngine { _, backend ->
            if (backend == OfflineBackend.GPU) gpu else cpu
        }

        val result = engine.initialize("/model")

        assertTrue(result.isFailure)
        assertTrue(result.exceptionOrNull()?.message.orEmpty().contains("GPU 初始化失败"))
        assertTrue(result.exceptionOrNull()?.message.orEmpty().contains("CPU 初始化失败"))
        assertTrue(gpu.closed)
        assertTrue(cpu.closed)
    }

    private class FakeHandle(
        private val initializeError: Throwable? = null,
    ) : LiteRtEngineHandle {
        override var isInitialized = false
        var closed = false

        override fun initialize() {
            isInitialized = true
            initializeError?.let { throw it }
        }

        override fun generate(
            systemPrompt: String,
            userPrompt: String,
            maxTokens: Int,
        ): Flow<String> = emptyFlow()

        override fun close() {
            closed = true
            isInitialized = false
        }
    }
}
