package com.example.orange.data.offline

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class OfflineCapabilityPreferencesTest {
    private val result = OfflineCapabilityResult(
        fingerprint = OfflineModelFingerprint(
            path = "/models/${OfflineModelSpec.FILE_NAME}",
            length = OfflineModelSpec.FILE_SIZE_BYTES,
            lastModified = 1234L,
            sha256 = OfflineModelSpec.SHA256,
        ),
        appVersionCode = 7L,
        backend = OfflineBackend.GPU,
        reply = "本地推理正常。",
        initializationMillis = 100L,
        firstTokenMillis = 20L,
        totalMillis = 150L,
    )

    @Test
    fun isCurrent_requiresMatchingFileMetadataAndAppVersion() {
        val metadata = FileMetadata(
            path = result.fingerprint.path,
            length = result.fingerprint.length,
            lastModified = result.fingerprint.lastModified,
        )

        assertTrue(result.isCurrent(metadata, 7L))
        assertFalse(result.isCurrent(metadata.copy(lastModified = 1235L), 7L))
        assertFalse(result.isCurrent(metadata, 8L))
        assertFalse(result.isCurrent(null, 7L))
    }

    @Test
    fun isCurrent_requiresPinnedHash() {
        val metadata = FileMetadata(
            path = result.fingerprint.path,
            length = result.fingerprint.length,
            lastModified = result.fingerprint.lastModified,
        )

        assertFalse(
            result.copy(
                fingerprint = result.fingerprint.copy(sha256 = "unexpected"),
            ).isCurrent(metadata, 7L),
        )
    }
}
