package com.example.orange.data.offline

import java.io.File
import java.io.RandomAccessFile
import java.nio.file.Files
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.assertThrows
import org.junit.Test

class OfflineCapabilityVerifierTest {
    @Test
    fun verify_acceptsNonEmptyReplyAndAlwaysClosesEngine() = runBlocking {
        val fixture = modelFixture()
        val clock = AtomicLong(0L)
        val engine = FakeEngine(
            initializeResult = Result.success(OfflineBackend.CPU),
            generated = flowOf("本地", "推理正常。"),
        )
        val stages = mutableListOf<OfflineCapabilityStage>()
        val verifier = verifier(
            manager = fixture.manager,
            engine = engine,
            clock = { clock.getAndAdd(10L) },
        )

        val result = verifier.verify(null, stages::add)

        assertEquals(OfflineBackend.CPU, result.backend)
        assertEquals("本地推理正常。", result.reply)
        assertEquals(10L, result.initializationMillis)
        assertEquals(10L, result.firstTokenMillis)
        assertTrue(engine.closed)
        assertEquals(
            listOf(
                OfflineCapabilityStage.CHECKING_FILE,
                OfflineCapabilityStage.VERIFYING_MODEL,
                OfflineCapabilityStage.CHECKING_DEVICE,
                OfflineCapabilityStage.INITIALIZING_ENGINE,
                OfflineCapabilityStage.TESTING_CONVERSATION,
            ),
            stages,
        )
        fixture.close()
    }

    @Test
    fun verify_rejectsEmptyReplyAndClosesEngine() {
        val fixture = modelFixture()
        val engine = FakeEngine(
            initializeResult = Result.success(OfflineBackend.GPU),
            generated = flowOf("  "),
        )

        val error = assertThrows(IllegalStateException::class.java) {
            runBlocking {
                verifier(fixture.manager, engine).verify(null, onStage = {})
            }
        }

        assertTrue(error.message.orEmpty().contains("空内容"))
        assertTrue(engine.closed)
        fixture.close()
    }

    @Test
    fun verify_propagatesInitializationFailureAndClosesEngine() {
        val fixture = modelFixture()
        val engine = FakeEngine(
            initializeResult = Result.failure(IllegalStateException("GPU 和 CPU 均失败")),
            generated = flowOf("unused"),
        )

        val error = assertThrows(IllegalStateException::class.java) {
            runBlocking {
                verifier(fixture.manager, engine).verify(null, onStage = {})
            }
        }

        assertEquals("GPU 和 CPU 均失败", error.message)
        assertTrue(engine.closed)
        fixture.close()
    }

    @Test
    fun verify_timesOutAndClosesEngine() {
        val fixture = modelFixture()
        val engine = FakeEngine(
            initializeResult = Result.success(OfflineBackend.GPU),
            generated = flow { awaitCancellation() },
        )
        val verifier = verifier(fixture.manager, engine, timeoutMillis = 20L)

        assertThrows(TimeoutCancellationException::class.java) {
            runBlocking { verifier.verify(null, onStage = {}) }
        }

        assertTrue(engine.closed)
        fixture.close()
    }

    @Test
    fun verify_cancellationClosesEngine() = runBlocking {
        val fixture = modelFixture()
        val generationStarted = CompletableDeferred<Unit>()
        val engine = FakeEngine(
            initializeResult = Result.success(OfflineBackend.GPU),
            generated = flow {
                generationStarted.complete(Unit)
                awaitCancellation()
            },
        )
        val verifier = verifier(fixture.manager, engine)

        val job = launch { verifier.verify(null, onStage = {}) }
        generationStarted.await()
        job.cancelAndJoin()

        assertTrue(engine.closed)
        fixture.close()
    }

    private fun verifier(
        manager: OfflineModelManager,
        engine: FakeEngine,
        clock: () -> Long = System::nanoTime,
        timeoutMillis: Long = 5_000L,
    ) = OfflineCapabilityVerifier(
        modelManager = manager,
        engineFactory = OfflineInferenceEngineFactory { engine },
        appVersionCode = 1L,
        elapsedRealtime = clock,
        timeoutMillis = timeoutMillis,
    )

    private fun modelFixture(): ModelFixture {
        val directory = Files.createTempDirectory("offline-verifier").toFile()
        RandomAccessFile(directory.resolve(OfflineModelSpec.FILE_NAME), "rw").use {
            it.setLength(OfflineModelSpec.FILE_SIZE_BYTES)
        }
        return ModelFixture(
            directory,
            OfflineModelManager(
                modelsDirectory = directory,
                deviceSnapshot = {
                    OfflineDeviceSnapshot(
                        supportedAbis = listOf("arm64-v8a"),
                        totalMemoryBytes = OfflineModelSpec.MIN_MEMORY_BYTES,
                    )
                },
                sha256 = { OfflineModelSpec.SHA256 },
            ),
        )
    }

    private data class ModelFixture(
        val directory: File,
        val manager: OfflineModelManager,
    ) {
        fun close() {
            directory.deleteRecursively()
        }
    }

    private class FakeEngine(
        private val initializeResult: Result<OfflineBackend>,
        private val generated: Flow<String>,
    ) : OfflineInferenceEngine {
        var closed = false

        override suspend fun initialize(modelPath: String): Result<OfflineBackend> = initializeResult

        override fun generate(
            systemPrompt: String,
            userPrompt: String,
            maxTokens: Int,
        ): Flow<String> = generated

        override suspend fun close() {
            closed = true
        }
    }
}
