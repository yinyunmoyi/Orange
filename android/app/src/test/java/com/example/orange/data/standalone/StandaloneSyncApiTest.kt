package com.example.orange.data.standalone

import com.example.orange.data.word.WordServiceConfig
import org.junit.Assert.assertEquals
import org.junit.Test

class StandaloneSyncApiTest {
    @Test
    fun candidateOrigins_prefersLocalAndRemovesDuplicates() {
        assertEquals(
            listOf("http://192.168.1.2:8888", WordServiceConfig.origin),
            StandaloneSyncApi.candidateOrigins("http://192.168.1.2:8888/"),
        )
        assertEquals(
            listOf(WordServiceConfig.origin),
            StandaloneSyncApi.candidateOrigins("${WordServiceConfig.origin}/"),
        )
    }
}
