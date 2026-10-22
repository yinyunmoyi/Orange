package com.example.orange.data.learning

import com.example.orange.data.logging.AppLog
import java.io.IOException
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

internal sealed interface LearningMutationDispatch {
    data class Success(val json: JSONObject) : LearningMutationDispatch
    data class DeliveryUnknown(
        val cause: Throwable,
        val canRetryAfterReconcile: Boolean,
    ) : LearningMutationDispatch
}

internal class LearningMutationCoordinator {
    private data class PendingReconciliation(
        val queueItemId: Long,
        val turnNo: Int,
        val cause: Throwable,
    )

    private val sessionMutexes = ConcurrentHashMap<Long, Mutex>()
    private val pendingReconciliations = ConcurrentHashMap<Long, PendingReconciliation>()

    suspend fun execute(
        sessionId: Long,
        expectedQueueItemId: Long,
        expectedTurnNo: Int,
        dispatch: suspend () -> LearningMutationDispatch,
        readCurrent: suspend () -> JSONObject,
        retryPublic: suspend () -> JSONObject,
    ): JSONObject = sessionMutexes.computeIfAbsent(sessionId) { Mutex() }.withLock {
        pendingReconciliations[sessionId]?.let { pending ->
            logState(sessionId, pending.queueItemId, pending.turnNo, "reconcile_resumed")
            val current = readCurrent()
            if (hasAdvanced(current, pending.queueItemId, pending.turnNo)) {
                pendingReconciliations.remove(sessionId)
                logState(sessionId, pending.queueItemId, pending.turnNo, "reconciled_advanced")
                return@withLock current
            }
            logState(sessionId, pending.queueItemId, pending.turnNo, "reconcile_blocked")
            throw IOException("learning mutation is still reconciling", pending.cause)
        }
        when (val result = dispatch()) {
            is LearningMutationDispatch.Success -> {
                pendingReconciliations.remove(sessionId)
                result.json
            }
            is LearningMutationDispatch.DeliveryUnknown -> {
                logState(sessionId, expectedQueueItemId, expectedTurnNo, "delivery_unknown")
                val pending = PendingReconciliation(
                    expectedQueueItemId,
                    expectedTurnNo,
                    result.cause,
                )
                val current = try {
                    readCurrent()
                } catch (error: Throwable) {
                    pendingReconciliations[sessionId] = pending
                    logState(
                        sessionId,
                        expectedQueueItemId,
                        expectedTurnNo,
                        "reconcile_read_failed",
                        error,
                    )
                    throw error
                }
                if (hasAdvanced(current, expectedQueueItemId, expectedTurnNo)) {
                    pendingReconciliations.remove(sessionId)
                    logState(sessionId, expectedQueueItemId, expectedTurnNo, "reconciled_advanced")
                    current
                } else if (result.canRetryAfterReconcile) {
                    logState(sessionId, expectedQueueItemId, expectedTurnNo, "retrying_public")
                    try {
                        retryPublic().also {
                            pendingReconciliations.remove(sessionId)
                            logState(
                                sessionId,
                                expectedQueueItemId,
                                expectedTurnNo,
                                "retry_public_completed",
                            )
                        }
                    } catch (error: LearningHttpStatusException) {
                        if (error.status != 409) {
                            pendingReconciliations[sessionId] = pending
                            throw error
                        }
                        val reconciled = readCurrent()
                        if (hasAdvanced(reconciled, expectedQueueItemId, expectedTurnNo)) {
                            pendingReconciliations.remove(sessionId)
                            logState(
                                sessionId,
                                expectedQueueItemId,
                                expectedTurnNo,
                                "conflict_reconciled",
                            )
                            reconciled
                        } else {
                            pendingReconciliations[sessionId] = pending
                            logState(
                                sessionId,
                                expectedQueueItemId,
                                expectedTurnNo,
                                "conflict_not_advanced",
                            )
                            throw IOException("learning mutation conflict did not advance state", error)
                        }
                    } catch (error: Throwable) {
                        pendingReconciliations[sessionId] = pending
                        logState(
                            sessionId,
                            expectedQueueItemId,
                            expectedTurnNo,
                            "retry_public_failed",
                            error,
                        )
                        throw error
                    }
                } else {
                    pendingReconciliations[sessionId] = pending
                    logState(sessionId, expectedQueueItemId, expectedTurnNo, "reconcile_blocked")
                    throw IOException("learning mutation delivery outcome is unknown", result.cause)
                }
            }
        }
    }

    internal fun hasAdvanced(
        current: JSONObject,
        expectedQueueItemId: Long,
        expectedTurnNo: Int,
    ): Boolean {
        val parsed = LearningApi.parseCurrent(current)
        return parsed.completed ||
            parsed.queueItemId != expectedQueueItemId ||
            parsed.turnNo != expectedTurnNo
    }

    private fun logState(
        sessionId: Long,
        queueItemId: Long,
        turnNo: Int,
        state: String,
        error: Throwable? = null,
    ) {
        AppLog.event(
            "LearningTrace",
            "mutation_$state",
            JSONObject()
                .put("sessionId", sessionId)
                .put("queueItemId", queueItemId)
                .put("turnNo", turnNo)
                .apply {
                    if (error != null) put("errorType", error.javaClass.name)
                },
            immediateUpload = true,
        )
    }
}

internal class LearningHttpStatusException(
    val status: Int,
    message: String,
) : IOException(message)
