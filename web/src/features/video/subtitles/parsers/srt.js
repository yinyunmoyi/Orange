import SrtParser from 'srt-parser-2'

const CJK_PATTERN = /[\u3040-\u30ff\u3400-\u9fff\uac00-\ud7af]/
const ASS_OVERRIDE_PATTERN = /\{\\[^}]*\}/g
const HTML_TAG_PATTERN = /<[^>]*>/g

export function parseSrt(text, { fileName = '' } = {}) {
  const parser = new SrtParser()
  const normalized = text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const parsed = parser.fromSrt(normalized)

  const normalizedCues = parsed
    .map((cue, index) => normalizeCue(cue, index))
    .filter((cue) => cue.sourceLines.length > 0)
    .filter((cue) => !isKnownPromotion(cue.sourceLines))
  const cues = mergeOverlappingAnnotations(normalizedCues)

  return {
    format: 'srt',
    fileName,
    cues,
  }
}

export function cleanSubtitleLine(line) {
  return line
    .replace(ASS_OVERRIDE_PATTERN, '')
    .replace(HTML_TAG_PATTERN, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function normalizeCue(cue, index) {
  const sourceLines = String(cue.text || '')
    .split('\n')
    .map(cleanSubtitleLine)
    .filter(Boolean)

  const primaryLines = []
  const translationLines = []
  for (const line of sourceLines) {
    if (CJK_PATTERN.test(line)) {
      translationLines.push(line)
    } else {
      primaryLines.push(line)
    }
  }

  return {
    id: `${cue.id || index + 1}-${index}`,
    startSeconds: Number(cue.startSeconds),
    endSeconds: Number(cue.endSeconds),
    primaryLines,
    translationLines,
    annotationLines: [],
    sourceLines,
  }
}

export function mergeOverlappingAnnotations(cues) {
  const dialogueByTimeline = new Map()
  for (const cue of cues) {
    if (!cue.primaryLines.length) continue
    dialogueByTimeline.set(timelineKey(cue), cue)
  }

  const annotationIds = new Set()
  for (const cue of cues) {
    if (cue.primaryLines.length || !cue.translationLines.length) continue
    const dialogue = dialogueByTimeline.get(timelineKey(cue))
    if (!dialogue) continue
    dialogue.annotationLines.push(...cue.translationLines)
    annotationIds.add(cue.id)
  }

  return cues.filter((cue) => !annotationIds.has(cue.id))
}

function timelineKey(cue) {
  return `${cue.startSeconds.toFixed(3)}:${cue.endSeconds.toFixed(3)}`
}

function isKnownPromotion(lines) {
  const text = lines.join(' ')
  return text.includes('添加微信公众号') && text.includes('ABContainer')
}
