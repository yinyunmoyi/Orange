import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildSubtitleLookupData,
  buildSubtitleParagraph,
  buildSubtitleSentenceData,
  unionRects,
} from './subtitleSelection.js'

function selected(text, start = 0, end = text.length) {
  return { text, selectedStart: start, selectedEnd: end }
}

test('buildSubtitleLookupData accepts words with apostrophes and hyphens', () => {
  for (const word of ['masked', "don't", 'well-known']) {
    const result = buildSubtitleLookupData([selected(word)])
    assert.equal(result.itemType, 'word')
    assert.equal(result.text, word)
    assert.equal(result.context.slice(result.wordStart, result.wordEnd), word)
  }
})

test('buildSubtitleLookupData creates a phrase within one cue', () => {
  const result = buildSubtitleLookupData([
    selected('Please look up this word.', 7, 14),
  ])

  assert.deepEqual(result, {
    text: 'look up',
    itemType: 'phrase',
    context: 'Please look up this word.',
    wordStart: 7,
    wordEnd: 14,
  })
})

test('buildSubtitleLookupData joins selected text across cues', () => {
  const result = buildSubtitleLookupData([
    selected('We should look', 10),
    selected('up this phrase', 0, 2),
  ])

  assert.equal(result.itemType, 'phrase')
  assert.equal(result.text, 'look up')
  assert.equal(result.context, 'We should look up this phrase')
  assert.equal(result.context.slice(result.wordStart, result.wordEnd), 'look up')
})

test('buildSubtitleLookupData normalizes whitespace and preserves offsets', () => {
  const result = buildSubtitleLookupData([
    selected('  We   should   look  ', 16, 20),
    selected('\tup   this phrase ', 1, 3),
  ])

  assert.equal(result.context, 'We should look up this phrase')
  assert.equal(result.text, 'look up')
  assert.equal(
    result.context.slice(result.wordStart, result.wordEnd),
    result.text,
  )
})

test('buildSubtitleLookupData accepts phrases up to eight tokens', () => {
  const phrase = 'one two three four five six seven eight'
  const result = buildSubtitleLookupData([selected(phrase)])

  assert.equal(result.itemType, 'phrase')
  assert.equal(result.text, phrase)
})

test('buildSubtitleLookupData rejects invalid selections', () => {
  const invalid = [
    '',
    'hello,',
    'word2',
    'one two three four five six seven eight nine',
    `one ${'a'.repeat(80)}`,
  ]

  for (const text of invalid) {
    assert.equal(buildSubtitleLookupData([selected(text)]), null)
  }
})

test('buildSubtitleLookupData follows document order for reversed selection data', () => {
  const result = buildSubtitleLookupData([
    selected('look', 0, 4),
    selected('up', 0, 2),
  ])

  assert.equal(result.text, 'look up')
  assert.equal(result.itemType, 'phrase')
})

test('buildSubtitleSentenceData accepts exact multi-word selections', () => {
  assert.deepEqual(
    buildSubtitleSentenceData([
      selected('Ignore this complete sentence.', 7),
    ]),
    { sentence: 'this complete sentence.' },
  )
  assert.deepEqual(
    buildSubtitleSentenceData([
      selected('We should analyze'),
      selected('this across cues.'),
    ]),
    { sentence: 'We should analyze this across cues.' },
  )
  assert.deepEqual(buildSubtitleSentenceData([selected('look up')]), {
    sentence: 'look up',
  })
})

test('buildSubtitleSentenceData normalizes whitespace and smart punctuation', () => {
  const result = buildSubtitleSentenceData([
    selected('  “We\u00a0really…  '),
    selected('should—analyze   this.”  '),
  ])

  assert.deepEqual(result, {
    sentence: '"We really... should-analyze this."',
  })
})

test('buildSubtitleSentenceData rejects words and enforces the length limit', () => {
  assert.equal(buildSubtitleSentenceData([selected('word')]), null)
  assert.equal(buildSubtitleSentenceData([selected('   ')]), null)
  assert.equal(buildSubtitleSentenceData([]), null)

  const atLimit = `one ${'a'.repeat(496)}`
  assert.equal(atLimit.length, 500)
  assert.equal(
    buildSubtitleSentenceData([selected(atLimit)])?.sentence.length,
    500,
  )
  assert.equal(
    buildSubtitleSentenceData([selected(`${atLimit}b`)]),
    null,
  )
})

test('buildSubtitleSentenceData keeps document segment order', () => {
  const result = buildSubtitleSentenceData([
    selected('first part'),
    selected('second part'),
  ])

  assert.equal(result.sentence, 'first part second part')
})

test('unionRects covers every selected subtitle line', () => {
  assert.deepEqual(
    unionRects([
      { left: 40, top: 20, right: 100, bottom: 36 },
      { left: 12, top: 44, right: 180, bottom: 62 },
    ]),
    {
      left: 12,
      top: 20,
      right: 180,
      bottom: 62,
      width: 168,
      height: 42,
    },
  )
  assert.equal(unionRects([]), null)
})

test('buildSubtitleParagraph creates UTF-16 cue ranges around selected cues', () => {
  const result = buildSubtitleParagraph({
    cues: [
      cue('before', 0, 2, ['😀 Before.']),
      cue('selected-1', 2, 5, ['Please look']),
      cue('selected-2', 5, 8, ['up this word.']),
      cue('after', 8, 10, ['After.'], ['之后。']),
    ],
    firstCueId: 'selected-1',
    lastCueId: 'selected-2',
  })

  assert.equal(
    result.paragraph,
    '😀 Before. Please look up this word. After.',
  )
  assert.deepEqual(
    result.cueRanges.map(({ cueId, textStart, textEnd }) => ({
      cueId,
      textStart,
      textEnd,
    })),
    [
      { cueId: 'before', textStart: 0, textEnd: 10 },
      { cueId: 'selected-1', textStart: 11, textEnd: 22 },
      { cueId: 'selected-2', textStart: 23, textEnd: 36 },
      { cueId: 'after', textStart: 37, textEnd: 43 },
    ],
  )
  assert.equal(result.paragraph.slice(18, 29), 'look up thi')
})

test('buildSubtitleParagraph excludes translations and respects context budget', () => {
  const result = buildSubtitleParagraph({
    cues: [
      cue('too-far-before', 0, 1, ['123456789']),
      cue('before', 1, 2, ['near']),
      cue('selected', 2, 3, ['target'], ['目标']),
      cue('after', 3, 4, ['close']),
      cue('too-far-after', 4, 5, ['123456789']),
    ],
    firstCueId: 'selected',
    lastCueId: 'selected',
    contextBudget: 6,
  })

  assert.equal(result.paragraph, 'near target close')
  assert.deepEqual(
    result.cueRanges.map((item) => item.cueId),
    ['before', 'selected', 'after'],
  )
  assert.equal(result.paragraph.includes('目标'), false)
})

test('buildSubtitleParagraph stops at large subtitle timeline gaps', () => {
  const result = buildSubtitleParagraph({
    cues: [
      cue('title', 0, 12.811, ['THE LAST FANTASY | TLF HALFCD TeaM']),
      cue('dialogue-1', 33.42, 36.571, [
        '"Notwithstanding the provisions',
      ]),
      cue('dialogue-2', 36.74, 42.27, ['clause 214 of the Act,']),
      cue('selected', 43.84, 47.175, [
        'the implementation of the statutory provisions is concerned,',
      ]),
      cue('dialogue-4', 47.24, 54.89, [
        'the resolution shall fall within the purview of the Minister".',
      ]),
      cue('later', 65, 68, ['A separate scene.']),
    ],
    firstCueId: 'selected',
    lastCueId: 'selected',
  })

  assert.deepEqual(
    result.cueRanges.map((item) => item.cueId),
    ['dialogue-1', 'dialogue-2', 'selected', 'dialogue-4'],
  )
  assert.equal(result.paragraph.includes('THE LAST FANTASY'), false)
  assert.equal(result.paragraph.includes('A separate scene.'), false)
})

function cue(id, startSeconds, endSeconds, primaryLines, translationLines = []) {
  return { id, startSeconds, endSeconds, primaryLines, translationLines }
}
