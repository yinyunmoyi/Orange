package com.example.orange.ui.learning

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AudioRegenerationActionTest {

    @Test
    fun reduceAudioRegenerationState_firstClickOnlyArms_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Closed,
            AudioRegenerationEvent.Click,
        )

        assertEquals(AudioRegenerationState.Armed, transition.state)
        assertFalse(transition.regenerate)
    }

    @Test
    fun reduceAudioRegenerationState_secondClickStartsRegeneration_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Armed,
            AudioRegenerationEvent.Click,
        )

        assertEquals(AudioRegenerationState.Regenerating, transition.state)
        assertTrue(transition.regenerate)
    }

    @Test
    fun reduceAudioRegenerationState_clickWhileRegeneratingIsIgnored_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Regenerating,
            AudioRegenerationEvent.Click,
        )

        assertEquals(AudioRegenerationState.Regenerating, transition.state)
        assertFalse(transition.regenerate)
    }

    @Test
    fun reduceAudioRegenerationState_failureKeepsActionArmed_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Regenerating,
            AudioRegenerationEvent.Failed,
        )

        assertEquals(AudioRegenerationState.Armed, transition.state)
        assertFalse(transition.regenerate)
    }

    @Test
    fun reduceAudioRegenerationState_successClosesAction_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Regenerating,
            AudioRegenerationEvent.Succeeded,
        )

        assertEquals(AudioRegenerationState.Closed, transition.state)
        assertFalse(transition.regenerate)
    }

    @Test
    fun reduceAudioRegenerationState_itemChangeClosesAction_BitsUT() {
        val transition = reduceAudioRegenerationState(
            AudioRegenerationState.Armed,
            AudioRegenerationEvent.ItemChanged,
        )

        assertEquals(AudioRegenerationState.Closed, transition.state)
        assertFalse(transition.regenerate)
    }
}
