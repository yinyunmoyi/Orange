package com.example.orange.data.learning

import java.io.IOException
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

class LearningMutationCoordinatorTest {
    @Test
    fun unknownDelivery_advancedCurrentIsTreatedAsSuccess() = runBlocking {
        val coordinator = LearningMutationCoordinator()
        var retried = false
        val advanced = current(queueItemId = 9L, turnNo = 4)

        val result = coordinator.execute(
            sessionId = 7L,
            expectedQueueItemId = 8L,
            expectedTurnNo = 3,
            dispatch = {
                LearningMutationDispatch.DeliveryUnknown(IOException("lost"), true)
            },
            readCurrent = { advanced },
            retryPublic = {
                retried = true
                current(10L, 5)
            },
        )

        assertSame(advanced, result)
        assertFalse(retried)
    }

    @Test
    fun unknownReverseDelivery_unchangedCurrentRetriesPublicOnce() = runBlocking {
        val coordinator = LearningMutationCoordinator()
        var retries = 0
        val retried = current(queueItemId = 9L, turnNo = 4)

        val result = coordinator.execute(
            sessionId = 7L,
            expectedQueueItemId = 8L,
            expectedTurnNo = 3,
            dispatch = {
                LearningMutationDispatch.DeliveryUnknown(IOException("lost"), true)
            },
            readCurrent = { current(8L, 3) },
            retryPublic = {
                retries++
                retried
            },
        )

        assertSame(retried, result)
        assertEquals(1, retries)
    }

    @Test
    fun execute_serializesMutationsForSameSession() = runBlocking {
        val coordinator = LearningMutationCoordinator()
        val firstStarted = CompletableDeferred<Unit>()
        val releaseFirst = CompletableDeferred<Unit>()
        val secondCalls = AtomicInteger()
        val first = async {
            coordinator.execute(
                sessionId = 7L,
                expectedQueueItemId = 8L,
                expectedTurnNo = 3,
                dispatch = {
                    firstStarted.complete(Unit)
                    releaseFirst.await()
                    LearningMutationDispatch.Success(current(9L, 4))
                },
                readCurrent = { error("not used") },
                retryPublic = { error("not used") },
            )
        }
        firstStarted.await()
        val second = async {
            coordinator.execute(
                sessionId = 7L,
                expectedQueueItemId = 9L,
                expectedTurnNo = 4,
                dispatch = {
                    secondCalls.incrementAndGet()
                    LearningMutationDispatch.Success(current(10L, 5))
                },
                readCurrent = { error("not used") },
                retryPublic = { error("not used") },
            )
        }

        delay(20L)
        assertEquals(0, secondCalls.get())
        releaseFirst.complete(Unit)
        first.await()
        second.await()
        assertEquals(1, secondCalls.get())
    }

    @Test
    fun unresolvedMutation_blocksLaterDispatchUntilCurrentAdvances() = runBlocking {
        val coordinator = LearningMutationCoordinator()
        var laterDispatches = 0
        val unchanged = current(8L, 3)
        val first = runCatching {
            coordinator.execute(
                sessionId = 7L,
                expectedQueueItemId = 8L,
                expectedTurnNo = 3,
                dispatch = {
                    LearningMutationDispatch.DeliveryUnknown(IOException("lost"), false)
                },
                readCurrent = { unchanged },
                retryPublic = { error("not used") },
            )
        }
        val second = runCatching {
            coordinator.execute(
                sessionId = 7L,
                expectedQueueItemId = 8L,
                expectedTurnNo = 3,
                dispatch = {
                    laterDispatches++
                    LearningMutationDispatch.Success(current(9L, 4))
                },
                readCurrent = { unchanged },
                retryPublic = { error("not used") },
            )
        }

        assertTrue(first.isFailure)
        assertTrue(second.isFailure)
        assertEquals(0, laterDispatches)
    }

    @Test
    fun hasAdvanced_detectsQueueTurnAndCompletionChanges() {
        val coordinator = LearningMutationCoordinator()

        assertFalse(coordinator.hasAdvanced(current(8L, 3), 8L, 3))
        assertTrue(coordinator.hasAdvanced(current(9L, 3), 8L, 3))
        assertTrue(coordinator.hasAdvanced(current(8L, 4), 8L, 3))
        assertTrue(coordinator.hasAdvanced(current(0L, 3, completed = true), 8L, 3))
    }

    private fun current(
        queueItemId: Long,
        turnNo: Int,
        completed: Boolean = false,
    ): JSONObject = JSONObject()
        .put(
            "data",
            JSONObject()
                .put("sessionId", 7L)
                .put("turnNo", turnNo)
                .put("queueItemId", queueItemId)
                .put("queueType", "review")
                .put("remaining", if (completed) 0 else 1)
                .put("completed", completed),
        )
}
