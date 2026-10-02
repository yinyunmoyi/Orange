package com.example.orange.data.offline

import android.app.ActivityManager
import android.content.Context
import android.os.Build
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.io.InputStream
import java.security.MessageDigest
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ensureActive
import kotlin.coroutines.coroutineContext

enum class OfflineValidationStage {
    CHECKING_FILE,
    VERIFYING_MODEL,
    CHECKING_DEVICE,
}

enum class OfflineValidationError {
    MODEL_DIRECTORY_UNAVAILABLE,
    MODEL_DIRECTORY_CREATE_FAILED,
    MODEL_MISSING,
    MODEL_FILE_NAME_MISMATCH,
    MODEL_NOT_READABLE,
    MODEL_LENGTH_MISMATCH,
    MODEL_HASH_MISMATCH,
    MODEL_IMPORT_OPEN_FAILED,
    MODEL_IMPORT_COPY_FAILED,
    UNSUPPORTED_ABI,
    INSUFFICIENT_MEMORY,
    INSUFFICIENT_STORAGE,
}

class OfflineValidationException(
    val error: OfflineValidationError,
    message: String,
) : IllegalStateException(message)

data class OfflineDeviceSnapshot(
    val supportedAbis: List<String>,
    val totalMemoryBytes: Long,
)

data class OfflineModelValidationMetrics(
    val hashCacheHit: Boolean,
    val hashMillis: Long,
    val supportedAbis: List<String>,
    val totalMemoryBytes: Long,
    val usableSpaceBytes: Long,
)

class OfflineModelManager(
    private val modelsDirectory: File?,
    private val deviceSnapshot: () -> OfflineDeviceSnapshot,
    private val sha256: (File) -> String = ::calculateSha256,
    private val usableSpaceBytes: (File) -> Long = File::getUsableSpace,
    private val expectedFileSizeBytes: Long = OfflineModelSpec.FILE_SIZE_BYTES,
    private val expectedSha256: String = OfflineModelSpec.SHA256,
) {
    fun modelFile(): File? = modelsDirectory?.resolve(OfflineModelSpec.FILE_NAME)

    fun ensureModelDirectory() {
        val directory = modelsDirectory ?: return
        if (!directory.exists()) directory.mkdirs()
    }

    suspend fun importModel(
        displayName: String?,
        declaredLength: Long?,
        openInputStream: () -> InputStream?,
        onStorageChecked: (usableBytes: Long, requiredBytes: Long) -> Unit = { _, _ -> },
        onProgress: (copiedBytes: Long, percent: Int) -> Unit = { _, _ -> },
    ): OfflineModelFingerprint {
        if (displayName != OfflineModelSpec.FILE_NAME) {
            fail(
                OfflineValidationError.MODEL_FILE_NAME_MISMATCH,
                "请选择文件 ${OfflineModelSpec.FILE_NAME}",
            )
        }
        if (declaredLength != null && declaredLength >= 0L &&
            declaredLength != expectedFileSizeBytes
        ) {
            fail(
                OfflineValidationError.MODEL_LENGTH_MISMATCH,
                "模型长度错误：$declaredLength，应为 $expectedFileSizeBytes",
            )
        }
        val directory = requireModelDirectory()
        val requiredSpaceBytes = expectedFileSizeBytes + OfflineModelSpec.MIN_FREE_SPACE_BYTES
        val availableSpaceBytes = usableSpaceBytes(directory)
        onStorageChecked(availableSpaceBytes, requiredSpaceBytes)
        if (availableSpaceBytes < requiredSpaceBytes) {
            fail(OfflineValidationError.INSUFFICIENT_STORAGE, "导入模型需要至少 2.5 GB 剩余空间")
        }

        val temporary = directory.resolve("${OfflineModelSpec.FILE_NAME}.importing")
        val backup = directory.resolve("${OfflineModelSpec.FILE_NAME}.backup")
        val destination = directory.resolve(OfflineModelSpec.FILE_NAME)
        temporary.delete()
        backup.delete()
        try {
            val digest = MessageDigest.getInstance("SHA-256")
            val input = openInputStream() ?: fail(
                OfflineValidationError.MODEL_IMPORT_OPEN_FAILED,
                "无法读取所选模型文件",
            )
            var copied = 0L
            var reportedPercent = 0
            onProgress(0L, 0)
            input.use { source ->
                FileOutputStream(temporary).use { output ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    while (true) {
                        coroutineContext.ensureActive()
                        val read = source.read(buffer)
                        if (read < 0) break
                        if (read == 0) continue
                        copied += read
                        if (copied > expectedFileSizeBytes) {
                            fail(OfflineValidationError.MODEL_LENGTH_MISMATCH, "模型文件大于预期长度")
                        }
                        digest.update(buffer, 0, read)
                        output.write(buffer, 0, read)
                        val percent = ((copied * 100L) / expectedFileSizeBytes)
                            .toInt()
                            .coerceAtMost(100)
                        if (percent >= reportedPercent + 10) {
                            reportedPercent = (percent / 10) * 10
                            onProgress(copied, reportedPercent)
                        }
                    }
                    output.fd.sync()
                }
            }
            if (copied != expectedFileSizeBytes) {
                fail(
                    OfflineValidationError.MODEL_LENGTH_MISMATCH,
                    "模型长度错误：$copied，应为 $expectedFileSizeBytes",
                )
            }
            val hash = digest.digest().toHex()
            if (hash != expectedSha256) {
                fail(OfflineValidationError.MODEL_HASH_MISMATCH, "模型 SHA-256 校验失败")
            }

            if (destination.exists() && !destination.renameTo(backup)) {
                fail(OfflineValidationError.MODEL_IMPORT_COPY_FAILED, "无法备份已有模型")
            }
            if (!temporary.renameTo(destination)) {
                if (backup.exists()) backup.renameTo(destination)
                fail(OfflineValidationError.MODEL_IMPORT_COPY_FAILED, "无法保存导入的模型")
            }
            backup.delete()
            return OfflineModelFingerprint(
                path = destination.absolutePath,
                length = destination.length(),
                lastModified = destination.lastModified(),
                sha256 = hash,
            )
        } catch (error: OfflineValidationException) {
            throw error
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: Throwable) {
            fail(
                OfflineValidationError.MODEL_IMPORT_COPY_FAILED,
                "导入模型失败：${error.message ?: error.javaClass.simpleName}",
            )
        } finally {
            temporary.delete()
        }
    }

    fun validate(
        cachedFingerprint: OfflineModelFingerprint?,
        onStage: (OfflineValidationStage) -> Unit = {},
        onMetrics: (OfflineModelValidationMetrics) -> Unit = {},
    ): OfflineModelFingerprint {
        onStage(OfflineValidationStage.CHECKING_FILE)
        val directory = requireModelDirectory()

        val model = directory.resolve(OfflineModelSpec.FILE_NAME)
        if (!model.exists()) {
            val otherModels = directory.listFiles()
                .orEmpty()
                .filter { it.isFile && it.extension.equals("litertlm", ignoreCase = true) }
            if (otherModels.isNotEmpty()) {
                fail(
                    OfflineValidationError.MODEL_FILE_NAME_MISMATCH,
                    "模型文件名不正确，应为 ${OfflineModelSpec.FILE_NAME}",
                )
            }
            fail(OfflineValidationError.MODEL_MISSING, "未找到模型文件：${model.path}")
        }
        if (!model.isFile || !model.canRead()) {
            fail(OfflineValidationError.MODEL_NOT_READABLE, "模型文件不可读：${model.path}")
        }
        if (model.length() != expectedFileSizeBytes) {
            fail(
                OfflineValidationError.MODEL_LENGTH_MISMATCH,
                "模型长度错误：${model.length()}，应为 $expectedFileSizeBytes",
            )
        }

        onStage(OfflineValidationStage.VERIFYING_MODEL)
        val currentPath = model.absolutePath
        val currentLength = model.length()
        val currentLastModified = model.lastModified()
        val hashCacheHit =
            cachedFingerprint?.path == currentPath &&
            cachedFingerprint.length == currentLength &&
            cachedFingerprint.lastModified == currentLastModified &&
            cachedFingerprint.sha256 == expectedSha256
        val hashStartNanos = System.nanoTime()
        val hash = if (hashCacheHit) {
            cachedFingerprint.sha256
        } else {
            sha256(model)
        }
        val hashMillis = (System.nanoTime() - hashStartNanos) / 1_000_000L
        if (!hash.equals(expectedSha256, ignoreCase = true)) {
            fail(OfflineValidationError.MODEL_HASH_MISMATCH, "模型 SHA-256 校验失败")
        }

        onStage(OfflineValidationStage.CHECKING_DEVICE)
        val device = deviceSnapshot()
        val availableSpaceBytes = usableSpaceBytes(directory)
        onMetrics(
            OfflineModelValidationMetrics(
                hashCacheHit = hashCacheHit,
                hashMillis = hashMillis,
                supportedAbis = device.supportedAbis,
                totalMemoryBytes = device.totalMemoryBytes,
                usableSpaceBytes = availableSpaceBytes,
            ),
        )
        if ("arm64-v8a" !in device.supportedAbis) {
            fail(OfflineValidationError.UNSUPPORTED_ABI, "离线模型仅支持 arm64-v8a 设备")
        }
        if (device.totalMemoryBytes < OfflineModelSpec.MIN_MEMORY_BYTES) {
            fail(OfflineValidationError.INSUFFICIENT_MEMORY, "设备物理内存不足 6 GB")
        }
        if (availableSpaceBytes < OfflineModelSpec.MIN_FREE_SPACE_BYTES) {
            fail(OfflineValidationError.INSUFFICIENT_STORAGE, "模型目录剩余空间不足 1.5 GB")
        }

        return OfflineModelFingerprint(
            path = currentPath,
            length = currentLength,
            lastModified = currentLastModified,
            sha256 = hash.lowercase(),
        )
    }

    private fun requireModelDirectory(): File {
        val directory = modelsDirectory ?: fail(
            OfflineValidationError.MODEL_DIRECTORY_UNAVAILABLE,
            "设备未提供 App 外部文件目录",
        )
        if (!directory.exists() && !directory.mkdirs()) {
            fail(OfflineValidationError.MODEL_DIRECTORY_CREATE_FAILED, "无法创建模型目录：${directory.path}")
        }
        if (!directory.isDirectory) {
            fail(OfflineValidationError.MODEL_DIRECTORY_CREATE_FAILED, "模型路径不是目录：${directory.path}")
        }
        return directory
    }

    companion object {
        fun create(context: Context): OfflineModelManager {
            val appContext = context.applicationContext
            return OfflineModelManager(
                modelsDirectory = appContext.getExternalFilesDir(null)?.resolve("models"),
                deviceSnapshot = {
                    val activityManager = appContext.getSystemService(ActivityManager::class.java)
                    val memoryInfo = ActivityManager.MemoryInfo()
                    activityManager.getMemoryInfo(memoryInfo)
                    OfflineDeviceSnapshot(
                        supportedAbis = Build.SUPPORTED_ABIS.toList(),
                        totalMemoryBytes = memoryInfo.totalMem,
                    )
                },
            )
        }
    }
}

private fun fail(error: OfflineValidationError, message: String): Nothing {
    throw OfflineValidationException(error, message)
}

private fun calculateSha256(file: File): String {
    val digest = MessageDigest.getInstance("SHA-256")
    FileInputStream(file).use { input ->
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        while (true) {
            val read = input.read(buffer)
            if (read < 0) break
            if (read > 0) digest.update(buffer, 0, read)
        }
    }
    return digest.digest().toHex()
}

private fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
