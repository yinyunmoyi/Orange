export function visibleSubtitleCues(cues, showTranslation) {
  return (Array.isArray(cues) ? cues : []).filter((cue) => {
    const hasPrimary = cue.primaryLines?.length > 0
    if (!showTranslation) return hasPrimary
    return (
      hasPrimary ||
      cue.translationLines?.length > 0 ||
      cue.annotationLines?.length > 0
    )
  })
}
