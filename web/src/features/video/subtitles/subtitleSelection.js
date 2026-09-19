const WORD_PATTERN = /^[A-Za-z][A-Za-z'-]*$/
const PHRASE_PATTERN =
  /^[A-Za-z][A-Za-z'-]*(?: [A-Za-z][A-Za-z'-]*){1,7}$/
const MAX_LOOKUP_LENGTH = 80
const MAX_SENTENCE_LENGTH = 500
const PARAGRAPH_CONTEXT_BUDGET = 2_000
const MAX_CONTEXT_GAP_SECONDS = 5

export function hasEnglishSubtitleSelection(selection, panel) {
  if (
    !selection ||
    selection.isCollapsed ||
    selection.rangeCount === 0 ||
    !selection.toString().trim() ||
    !panel
  ) {
    return false
  }

  const range = selection.getRangeAt(0)
  const englishNodes = panel.querySelectorAll(
    '[data-subtitle-text="primary"]',
  )

  return Array.from(englishNodes).some((node) => {
    try {
      return range.intersectsNode(node)
    } catch {
      return false
    }
  })
}

export function buildSubtitleLookupData(segments) {
  if (!Array.isArray(segments) || segments.length === 0) return null

  const normalizedSegments = segments
    .map((segment) => normalizeSegment(segment))
    .filter((segment) => segment.text)
  if (normalizedSegments.length === 0) return null

  let context = ''
  let wordStart = null
  let wordEnd = null

  normalizedSegments.forEach((segment) => {
    const separatorLength = context ? 1 : 0
    const segmentOffset = context.length + separatorLength
    context += `${separatorLength ? ' ' : ''}${segment.text}`

    if (segment.selectedStart == null || segment.selectedEnd == null) return
    if (wordStart == null) {
      wordStart = segmentOffset + segment.selectedStart
    }
    wordEnd = segmentOffset + segment.selectedEnd
  })

  if (wordStart == null || wordEnd == null || wordEnd <= wordStart) return null

  while (wordStart < wordEnd && /\s/.test(context[wordStart])) wordStart += 1
  while (wordEnd > wordStart && /\s/.test(context[wordEnd - 1])) wordEnd -= 1

  const text = context.slice(wordStart, wordEnd)
  const itemType = classifyLookupText(text)
  if (!itemType) return null

  return { text, itemType, context, wordStart, wordEnd }
}

export function buildSubtitleSentenceData(segments) {
  if (!Array.isArray(segments) || segments.length === 0) return null

  const sentence = normalizeSentenceText(
    segments
      .map((segment) => {
        const text = String(segment?.text || '')
        const start = Number.isInteger(segment?.selectedStart)
          ? segment.selectedStart
          : 0
        const end = Number.isInteger(segment?.selectedEnd)
          ? segment.selectedEnd
          : text.length
        return text.slice(
          Math.max(0, Math.min(start, text.length)),
          Math.max(0, Math.min(end, text.length)),
        )
      })
      .join(' '),
  )

  if (!sentence || sentence.length > MAX_SENTENCE_LENGTH) return null
  if (!/\S+\s+\S+/.test(sentence)) return null

  return { sentence }
}

export function createSubtitleLookupSnapshot(selection, panel, cues = []) {
  if (
    !selection ||
    selection.isCollapsed ||
    selection.rangeCount === 0 ||
    !panel
  ) {
    return null
  }

  const { lines, selectedLines } = collectSelectedEnglishLines(selection, panel)

  if (selectedLines.length === 0) return null

  const firstCueId = selectedLines[0].line.dataset.cueId
  const lastCueId = selectedLines[selectedLines.length - 1].line.dataset.cueId
  const firstIndex = lines.findIndex((line) => line.dataset.cueId === firstCueId)
  let lastIndex = lines.length - 1
  while (lastIndex >= 0 && lines[lastIndex].dataset.cueId !== lastCueId) {
    lastIndex -= 1
  }
  const selectedByLine = new Map(
    selectedLines.map(({ line, offsets }) => [line, offsets]),
  )
  const segments = lines.slice(firstIndex, lastIndex + 1).map((line) => {
    const offsets = selectedByLine.get(line)
    return {
      text: line.textContent || '',
      selectedStart: offsets?.start,
      selectedEnd: offsets?.end,
    }
  })
  const data = buildSubtitleLookupData(segments)
  if (!data) return null
  const paragraphData = buildSubtitleParagraph({
    cues,
    firstCueId,
    lastCueId,
  })
  if (!paragraphData) return null
  const firstCueRange = paragraphData.cueRanges.find(
    (cue) => cue.cueId === firstCueId,
  )
  if (!firstCueRange) return null
  const selectionStart = firstCueRange.textStart + data.wordStart
  const selectionEnd = firstCueRange.textStart + data.wordEnd
  const selectedText = paragraphData.paragraph.slice(
    selectionStart,
    selectionEnd,
  )
  if (normalizeText(selectedText) !== normalizeText(data.text)) return null

  const anchorRect = selectedLinesAnchorRect(selectedLines)
  if (!anchorRect) return null

  return {
    ...data,
    paragraph: paragraphData.paragraph,
    selectionStart,
    selectionEnd,
    cueRanges: paragraphData.cueRanges,
    anchorRect,
  }
}

export function createSubtitleSentenceSnapshot(selection, panel) {
  if (
    !selection ||
    selection.isCollapsed ||
    selection.rangeCount === 0 ||
    !panel
  ) {
    return null
  }

  const { selectedLines } = collectSelectedEnglishLines(selection, panel)
  if (selectedLines.length === 0) return null

  const data = buildSubtitleSentenceData(
    selectedLines.map(({ line, offsets }) => ({
      text: line.textContent || '',
      selectedStart: offsets.start,
      selectedEnd: offsets.end,
    })),
  )
  if (!data) return null

  const anchorRect = selectedLinesAnchorRect(selectedLines)
  if (!anchorRect) return null

  return { ...data, anchorRect }
}

export function buildSubtitleParagraph({
  cues,
  firstCueId,
  lastCueId,
  contextBudget = PARAGRAPH_CONTEXT_BUDGET,
}) {
  const englishCues = (Array.isArray(cues) ? cues : [])
    .map((cue) => ({
      cueId: String(cue.id),
      startSeconds: Number(cue.startSeconds),
      endSeconds: Number(cue.endSeconds),
      text: normalizeText((cue.primaryLines || []).join(' ')),
    }))
    .filter(
      (cue) =>
        cue.text &&
        Number.isFinite(cue.startSeconds) &&
        Number.isFinite(cue.endSeconds) &&
        cue.endSeconds > cue.startSeconds,
    )
  const firstIndex = englishCues.findIndex((cue) => cue.cueId === firstCueId)
  const lastIndex = englishCues.findIndex((cue) => cue.cueId === lastCueId)
  if (firstIndex < 0 || lastIndex < firstIndex) return null

  let startIndex = firstIndex
  let previousLength = 0
  while (startIndex > 0) {
    const previousCue = englishCues[startIndex - 1]
    const currentCue = englishCues[startIndex]
    if (
      currentCue.startSeconds - previousCue.endSeconds >
      MAX_CONTEXT_GAP_SECONDS
    ) {
      break
    }
    const addedLength = previousCue.text.length + 1
    if (previousLength + addedLength > contextBudget) break
    previousLength += addedLength
    startIndex -= 1
  }

  let endIndex = lastIndex
  let followingLength = 0
  while (endIndex < englishCues.length - 1) {
    const currentCue = englishCues[endIndex]
    const nextCue = englishCues[endIndex + 1]
    if (
      nextCue.startSeconds - currentCue.endSeconds >
      MAX_CONTEXT_GAP_SECONDS
    ) {
      break
    }
    const addedLength = nextCue.text.length + 1
    if (followingLength + addedLength > contextBudget) break
    followingLength += addedLength
    endIndex += 1
  }

  let paragraph = ''
  const cueRanges = englishCues
    .slice(startIndex, endIndex + 1)
    .map((cue) => {
      if (paragraph) paragraph += ' '
      const textStart = paragraph.length
      paragraph += cue.text
      return {
        ...cue,
        textStart,
        textEnd: paragraph.length,
      }
    })

  return { paragraph, cueRanges }
}

export function clearSubtitleSelection(selection = globalThis.getSelection?.()) {
  if (selection?.rangeCount) {
    selection.removeAllRanges()
  }
}

export function isPointInsideSubtitleText(container, clientX, clientY) {
  if (!container) return false

  return Array.from(container.querySelectorAll('[data-subtitle-text]')).some(
    (node) => {
      const rect = node.getBoundingClientRect()
      return (
        clientX >= rect.left &&
        clientX <= rect.right &&
        clientY >= rect.top &&
        clientY <= rect.bottom
      )
    },
  )
}

function classifyLookupText(text) {
  if (!text || text.length > MAX_LOOKUP_LENGTH) return null
  if (WORD_PATTERN.test(text)) return 'word'
  if (PHRASE_PATTERN.test(text)) return 'phrase'
  return null
}

function normalizeText(text) {
  return String(text || '').trim().replace(/\s+/g, ' ')
}

function normalizeSentenceText(text) {
  return normalizeText(text)
    .replace(/[\u2018\u2019]/g, "'")
    .replace(/[\u201c\u201d]/g, '"')
    .replace(/[\u2013\u2014]/g, '-')
    .replace(/\u2026/g, '...')
    .replace(/\u00a0/g, ' ')
}

function normalizeSegment(segment) {
  const rawText = String(segment?.text || '')
  const tokens = Array.from(rawText.matchAll(/\S+/g))
  if (tokens.length === 0) {
    return { text: '', selectedStart: null, selectedEnd: null }
  }

  const text = tokens.map((match) => match[0]).join(' ')
  const hasSelection =
    Number.isInteger(segment.selectedStart) &&
    Number.isInteger(segment.selectedEnd) &&
    segment.selectedEnd > segment.selectedStart

  return {
    text,
    selectedStart: hasSelection
      ? mapRawOffset(tokens, segment.selectedStart, text.length)
      : null,
    selectedEnd: hasSelection
      ? mapRawOffset(tokens, segment.selectedEnd, text.length)
      : null,
  }
}

function mapRawOffset(tokens, rawOffset, normalizedLength) {
  let normalizedOffset = 0

  for (const match of tokens) {
    const tokenStart = match.index
    const tokenEnd = tokenStart + match[0].length
    if (rawOffset <= tokenStart) return normalizedOffset
    if (rawOffset <= tokenEnd) {
      return normalizedOffset + rawOffset - tokenStart
    }
    normalizedOffset += match[0].length + 1
  }

  return normalizedLength
}

function collectSelectedEnglishLines(selection, panel) {
  if (!selection?.rangeCount || !panel) {
    return { lines: [], selectedLines: [] }
  }
  const range = selection.getRangeAt(0)
  const lines = Array.from(
    panel.querySelectorAll('[data-subtitle-line="primary"]'),
  )
  const selectedLines = lines
    .map((line, index) => ({
      index,
      line,
      offsets: selectedOffsets(range, line),
    }))
    .filter(({ offsets }) => offsets && offsets.end > offsets.start)
  return { lines, selectedLines }
}

function selectedLinesAnchorRect(selectedLines) {
  const rects = selectedLines.flatMap(({ line, offsets }) =>
    selectedClientRects(line, offsets),
  )
  return unionRects(rects)
}

function selectedOffsets(range, line) {
  try {
    if (!range.intersectsNode(line)) return null
  } catch {
    return null
  }

  const length = (line.textContent || '').length
  const start = line.contains(range.startContainer)
    ? textOffsetWithin(line, range.startContainer, range.startOffset)
    : 0
  const end = line.contains(range.endContainer)
    ? textOffsetWithin(line, range.endContainer, range.endOffset)
    : length

  return {
    start: Math.max(0, Math.min(start, length)),
    end: Math.max(0, Math.min(end, length)),
  }
}

function textOffsetWithin(element, container, offset) {
  const prefix = element.ownerDocument.createRange()
  prefix.selectNodeContents(element)
  prefix.setEnd(container, offset)
  return prefix.toString().length
}

function selectedClientRects(line, offsets) {
  const document = line.ownerDocument
  const range = document.createRange()
  const start = textBoundary(line, offsets.start)
  const end = textBoundary(line, offsets.end)
  range.setStart(start.node, start.offset)
  range.setEnd(end.node, end.offset)

  const rects = Array.from(range.getClientRects()).filter(
    (rect) => rect.width > 0 && rect.height > 0,
  )
  if (rects.length > 0) return rects

  const fallback = range.getBoundingClientRect()
  return fallback.width > 0 && fallback.height > 0 ? [fallback] : []
}

function textBoundary(element, targetOffset) {
  const document = element.ownerDocument
  const walker = document.createTreeWalker(
    element,
    globalThis.NodeFilter?.SHOW_TEXT || 4,
  )
  let remaining = targetOffset
  let node = walker.nextNode()

  while (node) {
    const length = node.nodeValue?.length || 0
    if (remaining <= length) return { node, offset: remaining }
    remaining -= length
    node = walker.nextNode()
  }

  return { node: element, offset: element.childNodes.length }
}

export function unionRects(rects) {
  if (!rects.length) return null

  const left = Math.min(...rects.map((rect) => rect.left))
  const top = Math.min(...rects.map((rect) => rect.top))
  const right = Math.max(...rects.map((rect) => rect.right))
  const bottom = Math.max(...rects.map((rect) => rect.bottom))

  return {
    left,
    top,
    right,
    bottom,
    width: right - left,
    height: bottom - top,
  }
}
