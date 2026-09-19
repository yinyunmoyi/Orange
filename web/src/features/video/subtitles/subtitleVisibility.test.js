import assert from 'node:assert/strict'
import test from 'node:test'
import { visibleSubtitleCues } from './subtitleVisibility.js'

const english = {
  id: 'english',
  primaryLines: ['Hello.'],
  translationLines: ['你好。'],
  annotationLines: [],
}
const translationOnly = {
  id: 'translation-only',
  primaryLines: [],
  translationLines: ['仅中文。'],
  annotationLines: [],
}

test('only-English mode removes translation-only cues', () => {
  assert.deepEqual(
    visibleSubtitleCues([english, translationOnly], false).map(
      (cue) => cue.id,
    ),
    ['english'],
  )
})

test('bilingual mode keeps translation-only cues', () => {
  assert.deepEqual(
    visibleSubtitleCues([english, translationOnly], true).map(
      (cue) => cue.id,
    ),
    ['english', 'translation-only'],
  )
})
