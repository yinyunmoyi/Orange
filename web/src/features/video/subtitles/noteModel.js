export const MAX_NOTE_CODE_POINTS = 2000

export function noteCodePointCount(value) {
  return Array.from(value).length
}

export function canSaveNote(value, saving) {
  return !saving && noteCodePointCount(value) <= MAX_NOTE_CODE_POINTS
}
