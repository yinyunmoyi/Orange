package com.example.orange.ui.wordlist

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class FavoriteDeleteActionTest {

    @Test
    fun reduceFavoriteDeleteState_firstClickOnlyArms_BitsUT() {
        val transition = reduceFavoriteDeleteState(
            FavoriteDeleteState.Closed,
            FavoriteDeleteEvent.Click,
        )

        assertEquals(FavoriteDeleteState.Armed, transition.state)
        assertFalse(transition.confirmDelete)
    }

    @Test
    fun reduceFavoriteDeleteState_secondClickConfirmsDelete_BitsUT() {
        val transition = reduceFavoriteDeleteState(
            FavoriteDeleteState.Armed,
            FavoriteDeleteEvent.Click,
        )

        assertEquals(FavoriteDeleteState.Deleting, transition.state)
        assertTrue(transition.confirmDelete)
    }

    @Test
    fun reduceFavoriteDeleteState_failureKeepsActionArmed_BitsUT() {
        val transition = reduceFavoriteDeleteState(
            FavoriteDeleteState.Deleting,
            FavoriteDeleteEvent.DeleteFailed,
        )

        assertEquals(FavoriteDeleteState.Armed, transition.state)
        assertFalse(transition.confirmDelete)
    }

    @Test
    fun reduceFavoriteDeleteState_itemChangeClosesAction_BitsUT() {
        val transition = reduceFavoriteDeleteState(
            FavoriteDeleteState.Armed,
            FavoriteDeleteEvent.ItemChanged,
        )

        assertEquals(FavoriteDeleteState.Closed, transition.state)
        assertFalse(transition.confirmDelete)
    }

    @Test
    fun reduceFavoriteDeleteState_clickWhileDeletingIsIgnored_BitsUT() {
        val transition = reduceFavoriteDeleteState(
            FavoriteDeleteState.Deleting,
            FavoriteDeleteEvent.Click,
        )

        assertEquals(FavoriteDeleteState.Deleting, transition.state)
        assertFalse(transition.confirmDelete)
    }
}
