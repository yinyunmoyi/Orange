package com.example.orange.data.reverselan

import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test

class ReverseLanQueueTest {
    @Test
    fun execute_dispatchedCommandCompletesWithMatchingResponse() = runBlocking {
        val queue = ReverseLanQueue(responseTimeoutMillis = 1_000L)
        val request = request()
        val pending = async(start = CoroutineStart.UNDISPATCHED) { queue.execute(request) }

        assertEquals(request, queue.next(REVERSE_LAN_INTERACTIVE_LANE, 100L))
        val response = response("{}")
        assertEquals(ReverseLanQueue.CompletionStatus.ACCEPTED, queue.complete(response))
        assertEquals(ReverseLanResult.Response(response), pending.await())
        assertEquals(ReverseLanQueue.CompletionStatus.DUPLICATE, queue.complete(response))
        assertEquals(
            ReverseLanQueue.CompletionStatus.CONFLICT,
            queue.complete(response("""{"code":0}""")),
        )
    }

    @Test
    fun next_deliversEachRequestAtMostOnce() = runBlocking {
        val queue = ReverseLanQueue(responseTimeoutMillis = 1_000L)
        val pending = async(start = CoroutineStart.UNDISPATCHED) { queue.execute(request()) }

        queue.next(REVERSE_LAN_INTERACTIVE_LANE, 100L)

        assertNull(queue.next(REVERSE_LAN_INTERACTIVE_LANE, 10L))
        queue.stop()
        assertSame(ReverseLanResult.DeliveryUnknown, pending.await())
    }

    @Test
    fun stop_returnsNotDispatchedForQueuedRequest() = runBlocking {
        val queue = ReverseLanQueue(responseTimeoutMillis = 1_000L)
        val pending = async(start = CoroutineStart.UNDISPATCHED) { queue.execute(request()) }

        queue.stop()

        assertSame(ReverseLanResult.NotDispatched, pending.await())
    }

    @Test(timeout = 1_000L)
    fun execute_queuedTimeoutReturnsNotDispatched() = runBlocking {
        val queue = ReverseLanQueue(responseTimeoutMillis = 20L)

        val result = queue.execute(request())

        assertSame(ReverseLanResult.NotDispatched, result)
    }

    @Test(timeout = 1_000L)
    fun execute_dispatchedTimeoutReturnsDeliveryUnknown() = runBlocking {
        val queue = ReverseLanQueue(responseTimeoutMillis = 20L)
        val pending = async(start = CoroutineStart.UNDISPATCHED) { queue.execute(request()) }
        queue.next(REVERSE_LAN_INTERACTIVE_LANE, 10L)

        assertSame(ReverseLanResult.DeliveryUnknown, pending.await())
    }

    private fun request(): ReverseLanRequest = ReverseLanRequest(
        requestId = "11111111-1111-4111-8111-111111111111",
        lane = REVERSE_LAN_INTERACTIVE_LANE,
        method = "POST",
        path = "/api/v1/learning/sessions/8/answers",
        headers = mapOf("Content-Type" to "application/json"),
        body = "{}",
    )

    private fun response(body: String): ReverseLanResponse = ReverseLanResponse(
        requestId = "11111111-1111-4111-8111-111111111111",
        status = 200,
        headers = mapOf("Content-Type" to "application/json"),
        body = body,
    )
}
