package com.example.orange.data.offline

import java.io.ByteArrayInputStream
import java.io.File
import java.io.RandomAccessFile
import java.nio.file.Files
import java.security.MessageDigest
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

class OfflineModelManagerTest {
    @Test
    fun sanitizeOfflineErrorMessage_removesUrisAndAbsolutePaths() {
        val sanitized = sanitizeOfflineErrorMessage(
            "failed content://provider/model at /storage/emulated/0/Download/model.litertlm: GPU unsupported",
        )

        assertFalse(sanitized.contains("content://"))
        assertFalse(sanitized.contains("/storage/"))
        assertTrue(sanitized.contains("GPU unsupported"))
    }

    @Test
    fun validate_acceptsExpectedModelAndReusesMatchingHash() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        val model = createExpectedLengthModel(directory)
        var hashCalls = 0
        val manager = manager(directory) {
            hashCalls++
            OfflineModelSpec.SHA256
        }
        var metrics: OfflineModelValidationMetrics? = null
        val cached = OfflineModelFingerprint(
            path = model.absolutePath,
            length = model.length(),
            lastModified = model.lastModified(),
            sha256 = OfflineModelSpec.SHA256,
        )

        val result = manager.validate(cached, onMetrics = { metrics = it })

        assertEquals(cached, result)
        assertEquals(0, hashCalls)
        assertEquals(true, metrics?.hashCacheHit)
        assertEquals(listOf("arm64-v8a"), metrics?.supportedAbis)
        directory.deleteRecursively()
    }

    @Test
    fun validate_recomputesHashWhenFileMetadataChanges() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        val model = createExpectedLengthModel(directory)
        var hashCalls = 0
        val manager = manager(directory) {
            hashCalls++
            OfflineModelSpec.SHA256
        }
        val stale = OfflineModelFingerprint(
            path = model.absolutePath,
            length = model.length(),
            lastModified = model.lastModified() - 1L,
            sha256 = OfflineModelSpec.SHA256,
        )

        manager.validate(stale)

        assertEquals(1, hashCalls)
        directory.deleteRecursively()
    }

    @Test
    fun validate_reportsWrongModelFileName() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        RandomAccessFile(directory.resolve("wrong.litertlm"), "rw").use {
            it.setLength(OfflineModelSpec.FILE_SIZE_BYTES)
        }
        val error = assertThrows(OfflineValidationException::class.java) {
            manager(directory).validate(null)
        }

        assertEquals(OfflineValidationError.MODEL_FILE_NAME_MISMATCH, error.error)
        directory.deleteRecursively()
    }

    @Test
    fun validate_rejectsWrongLengthAndHash() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        directory.resolve(OfflineModelSpec.FILE_NAME).writeText("invalid")
        val lengthError = assertThrows(OfflineValidationException::class.java) {
            manager(directory).validate(null)
        }
        assertEquals(OfflineValidationError.MODEL_LENGTH_MISMATCH, lengthError.error)

        createExpectedLengthModel(directory)
        val hashError = assertThrows(OfflineValidationException::class.java) {
            manager(directory) { "unexpected" }.validate(null)
        }
        assertEquals(OfflineValidationError.MODEL_HASH_MISMATCH, hashError.error)
        directory.deleteRecursively()
    }

    @Test
    fun validate_rejectsUnsupportedAbiAndInsufficientMemory() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        createExpectedLengthModel(directory)
        val unsupported = OfflineModelManager(
            modelsDirectory = directory,
            deviceSnapshot = { OfflineDeviceSnapshot(listOf("x86_64"), Long.MAX_VALUE) },
            sha256 = { OfflineModelSpec.SHA256 },
        )
        val abiError = assertThrows(OfflineValidationException::class.java) {
            unsupported.validate(null)
        }
        assertEquals(OfflineValidationError.UNSUPPORTED_ABI, abiError.error)

        val lowMemory = OfflineModelManager(
            modelsDirectory = directory,
            deviceSnapshot = {
                OfflineDeviceSnapshot(listOf("arm64-v8a"), OfflineModelSpec.MIN_MEMORY_BYTES - 1L)
            },
            sha256 = { OfflineModelSpec.SHA256 },
        )
        val memoryError = assertThrows(OfflineValidationException::class.java) {
            lowMemory.validate(null)
        }
        assertEquals(OfflineValidationError.INSUFFICIENT_MEMORY, memoryError.error)
        directory.deleteRecursively()
    }

    @Test
    fun validate_rejectsInsufficientRemainingStorage() {
        val directory = Files.createTempDirectory("offline-model").toFile()
        createExpectedLengthModel(directory)
        val manager = OfflineModelManager(
            modelsDirectory = directory,
            deviceSnapshot = {
                OfflineDeviceSnapshot(listOf("arm64-v8a"), OfflineModelSpec.MIN_MEMORY_BYTES)
            },
            sha256 = { OfflineModelSpec.SHA256 },
            usableSpaceBytes = { OfflineModelSpec.MIN_FREE_SPACE_BYTES - 1L },
        )

        val error = assertThrows(OfflineValidationException::class.java) {
            manager.validate(null)
        }

        assertEquals(OfflineValidationError.INSUFFICIENT_STORAGE, error.error)
        directory.deleteRecursively()
    }

    @Test
    fun importModel_atomicallyCopiesValidatedLocalSendFile() = runBlocking {
        val directory = Files.createTempDirectory("offline-import").toFile()
        val bytes = "valid-model".toByteArray()
        val manager = importManager(directory, bytes)
        val progress = mutableListOf<Int>()

        val result = manager.importModel(
            displayName = OfflineModelSpec.FILE_NAME,
            declaredLength = bytes.size.toLong(),
            openInputStream = { ByteArrayInputStream(bytes) },
            onProgress = { _, percent -> progress += percent },
        )

        assertArrayEquals(bytes, File(result.path).readBytes())
        assertEquals(bytes.size.toLong(), result.length)
        assertEquals(listOf(0, 100), progress)
        directory.deleteRecursively()
        Unit
    }

    @Test
    fun importModel_keepsExistingModelWhenHashIsInvalid() = runBlocking {
        val directory = Files.createTempDirectory("offline-import").toFile()
        val expected = "valid-model".toByteArray()
        val existing = directory.resolve(OfflineModelSpec.FILE_NAME)
        existing.writeText("existing")
        val manager = importManager(directory, expected)

        val error = assertThrows(OfflineValidationException::class.java) {
            runBlocking {
                manager.importModel(
                    displayName = OfflineModelSpec.FILE_NAME,
                    declaredLength = expected.size.toLong(),
                    openInputStream = { ByteArrayInputStream("badly-model".toByteArray()) },
                )
            }
        }

        assertEquals(OfflineValidationError.MODEL_HASH_MISMATCH, error.error)
        assertEquals("existing", existing.readText())
        directory.deleteRecursively()
        Unit
    }

    private fun manager(
        directory: File,
        sha256: (File) -> String = { OfflineModelSpec.SHA256 },
    ) = OfflineModelManager(
        modelsDirectory = directory,
        deviceSnapshot = {
            OfflineDeviceSnapshot(listOf("arm64-v8a"), OfflineModelSpec.MIN_MEMORY_BYTES)
        },
        sha256 = sha256,
    )

    private fun createExpectedLengthModel(directory: File): File =
        directory.resolve(OfflineModelSpec.FILE_NAME).also { model ->
            RandomAccessFile(model, "rw").use {
                it.setLength(OfflineModelSpec.FILE_SIZE_BYTES)
            }
        }

    private fun importManager(directory: File, expectedBytes: ByteArray) = OfflineModelManager(
        modelsDirectory = directory,
        deviceSnapshot = {
            OfflineDeviceSnapshot(listOf("arm64-v8a"), OfflineModelSpec.MIN_MEMORY_BYTES)
        },
        usableSpaceBytes = { Long.MAX_VALUE },
        expectedFileSizeBytes = expectedBytes.size.toLong(),
        expectedSha256 = MessageDigest.getInstance("SHA-256")
            .digest(expectedBytes)
            .joinToString("") { "%02x".format(it) },
    )
}
