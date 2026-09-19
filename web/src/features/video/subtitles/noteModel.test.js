import assert from 'node:assert/strict'
import test from 'node:test'
import {
  MAX_NOTE_CODE_POINTS,
  canSaveNote,
  noteCodePointCount,
} from './noteModel.js'

test('noteCodePointCount counts ASCII, Chinese, and emoji code points', () => {
  assert.equal(noteCodePointCount('note'), 4)
  assert.equal(noteCodePointCount('备注'), 2)
  assert.equal(noteCodePointCount('A😀中'), 3)
})

test('canSaveNote accepts empty and exactly 2000 emoji characters', () => {
  assert.equal(canSaveNote('', false), true)
  assert.equal(
    canSaveNote('😀'.repeat(MAX_NOTE_CODE_POINTS), false),
    true,
  )
})

test('canSaveNote rejects over-limit text and saving state', () => {
  assert.equal(
    canSaveNote('😀'.repeat(MAX_NOTE_CODE_POINTS + 1), false),
    false,
  )
  assert.equal(canSaveNote('note', true), false)
})
