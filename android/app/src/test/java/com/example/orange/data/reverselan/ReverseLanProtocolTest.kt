package com.example.orange.data.reverselan

import java.io.ByteArrayInputStream
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class ReverseLanProtocolTest {
    @Test
    fun requestRoundTrip_preservesAllowedFields() {
        val request = ReverseLanRequest(
            requestId = "11111111-1111-4111-8111-111111111111",
            lane = REVERSE_LAN_INTERACTIVE_LANE,
            method = "POST",
            path = "/api/v1/learning/sessions/8/answers",
            headers = mapOf(
                "Content-Type" to "application/json",
                "X-Orange-Debug-Trace-Id" to "trace-one",
            ),
            body = "{}",
        )

        val parsed = ReverseLanRequest.fromJson(request.toJson())

        assertEquals(request, parsed)
    }

    @Test
    fun requestValidation_rejectsAbsoluteUrlAndSensitiveHeader() {
        assertThrows(IllegalArgumentException::class.java) {
            validRequest().copy(path = "http://192.168.1.1/api/v1/private").validate()
        }
        assertThrows(IllegalArgumentException::class.java) {
            validRequest().copy(headers = mapOf("Authorization" to "secret")).validate()
        }
    }

    @Test
    fun requestParsing_rejectsUnknownFields() {
        val json = validRequest().toJson().put("host", "192.168.1.1")

        assertThrows(IllegalArgumentException::class.java) {
            ReverseLanRequest.fromJson(json)
        }
    }

    @Test
    fun responseHash_isStableAndIncludesBody() {
        val first = validResponse("{}")
        val same = ReverseLanResponse.fromJson(
            JSONObject()
                .put("body", "{}")
                .put("headers", JSONObject().put("Content-Type", "application/json"))
                .put("status", 200)
                .put("requestId", first.requestId)
                .put("apiVersion", 1),
        )
        val different = validResponse("""{"code":0}""")

        assertEquals(first.stableHash(), same.stableHash())
        org.junit.Assert.assertNotEquals(first.stableHash(), different.stableHash())
    }

    @Test
    fun relayResultBody_preservesAllLearningTextFieldsAsUtf8() {
        val card = JSONObject()
            .put("phonetic", "/ˈpʌfəri/")
            .put("chinese", "吹捧、夸大宣传；吹嘘、虚夸")
            .put("sentence", "She called it “mere puffery”—not evidence.")
            .put("translation", "她称其为“纯粹吹嘘”，而非证据。")
            .put("note", "区分：puffery ≠ 欺诈")
        val response = JSONObject()
            .put("apiVersion", 1)
            .put("requestId", "11111111-1111-4111-8111-111111111111")
            .put("status", 200)
            .put("headers", JSONObject().put("Content-Type", "application/json; charset=utf-8"))
            .put("body", JSONObject().put("data", card).toString())
        val bytes = response.toString().toByteArray(Charsets.UTF_8)

        val decoded = ReverseLanServer.readUtf8Body(
            ByteArrayInputStream(bytes),
            bytes.size,
            bytes.size,
        )
        val parsedCard = JSONObject(ReverseLanResponse.fromJson(JSONObject(decoded)).body)
            .getJSONObject("data")

        assertEquals("/ˈpʌfəri/", parsedCard.getString("phonetic"))
        assertEquals("吹捧、夸大宣传；吹嘘、虚夸", parsedCard.getString("chinese"))
        assertEquals(
            "She called it “mere puffery”—not evidence.",
            parsedCard.getString("sentence"),
        )
        assertEquals("她称其为“纯粹吹嘘”，而非证据。", parsedCard.getString("translation"))
        assertEquals("区分：puffery ≠ 欺诈", parsedCard.getString("note"))
        org.junit.Assert.assertFalse(decoded.contains('\uFFFD'))
    }

    private fun validRequest(): ReverseLanRequest = ReverseLanRequest(
        requestId = "11111111-1111-4111-8111-111111111111",
        lane = REVERSE_LAN_INTERACTIVE_LANE,
        method = "GET",
        path = "/api/v1/learning/sessions/8/current",
    )

    private fun validResponse(body: String): ReverseLanResponse = ReverseLanResponse(
        requestId = "11111111-1111-4111-8111-111111111111",
        status = 200,
        headers = mapOf("Content-Type" to "application/json"),
        body = body,
    )
}
