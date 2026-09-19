import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { ScanText, Search } from 'lucide-react'
import {
  createContextTask,
  uploadContextVideo,
} from '../../../api/word.js'
import { buildContextClip } from '../context/contextClipModel.js'
import { recordContextClip } from '../context/recordContextClip.js'
import {
  clearSubtitleSelection,
  createSubtitleLookupSnapshot,
  createSubtitleSentenceSnapshot,
  hasEnglishSubtitleSelection,
  isPointInsideSubtitleText,
} from './subtitleSelection.js'
import { findActiveCues, findHighlightedCue } from './subtitleTiming.js'
import { visibleSubtitleCues } from './subtitleVisibility.js'
import SubtitleLookupPopover from './SubtitleLookupPopover.jsx'
import SubtitleSentenceAnalysisPopover from './SubtitleSentenceAnalysisPopover.jsx'

export default function SubtitlePanel({
  cues,
  currentTime,
  onSeek,
  showTranslation = true,
  videoUrl,
  playbackSource,
  sourceFingerprint,
  onSentenceAnalysisOpen,
}) {
  const cueRefs = useRef(new Map())
  const panelRef = useRef(null)
  const listRef = useRef(null)
  const selectionActionRef = useRef(null)
  const popoverRef = useRef(null)
  const contextJobsRef = useRef(new Map())
  const [hasSelection, setHasSelection] = useState(false)
  const [selectionCandidate, setSelectionCandidate] = useState(null)
  const [activeAction, setActiveAction] = useState(null)
  const [, setViewportTick] = useState(0)
  const visibleCues = useMemo(
    () => visibleSubtitleCues(cues, showTranslation),
    [cues, showTranslation],
  )
  const highlightedCue = useMemo(
    () => findHighlightedCue(visibleCues, currentTime),
    [visibleCues, currentTime],
  )
  const activeIds = useMemo(
    () => new Set(findActiveCues(visibleCues, currentTime).map((cue) => cue.id)),
    [visibleCues, currentTime],
  )
  const preferredCueId = highlightedCue?.id

  useEffect(() => {
    const handleSelectionChange = () => {
      const selection = window.getSelection()
      const hasEnglishSelection = hasEnglishSubtitleSelection(
        selection,
        panelRef.current,
      )
      const lookup = hasEnglishSelection
        ? createSubtitleLookupSnapshot(selection, panelRef.current, cues)
        : null
      const sentence = hasEnglishSelection
        ? createSubtitleSentenceSnapshot(selection, panelRef.current)
        : null
      const candidate =
        lookup || sentence
          ? {
              lookup,
              sentence,
              anchorRect: sentence?.anchorRect || lookup?.anchorRect,
            }
          : null
      setHasSelection(!!candidate)
      setSelectionCandidate(candidate)
      if (candidate) setActiveAction(null)
    }

    document.addEventListener('selectionchange', handleSelectionChange)
    return () => {
      document.removeEventListener('selectionchange', handleSelectionChange)
      clearSubtitleSelection()
    }
  }, [cues])

  useEffect(() => {
    const jobs = contextJobsRef.current
    return () => {
      jobs.forEach((job) => job.controller.abort())
      jobs.clear()
    }
  }, [])

  useEffect(() => {
    clearSubtitleSelection()
    setHasSelection(false)
    setSelectionCandidate(null)
    setActiveAction(null)
  }, [showTranslation])

  useEffect(() => {
    const handleResize = () => setViewportTick((value) => value + 1)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  useEffect(() => {
    if (!activeAction) return undefined

    const dismiss = () => {
      clearSubtitleSelection()
      setHasSelection(false)
      setSelectionCandidate(null)
      setActiveAction(null)
    }
    const handlePointerDown = (event) => {
      if (
        popoverRef.current?.contains(event.target) ||
        selectionActionRef.current?.contains(event.target)
      ) {
        return
      }
      dismiss()
    }
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') dismiss()
    }

    document.addEventListener('pointerdown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [activeAction])

  useLayoutEffect(() => {
    if (!preferredCueId || hasSelection || activeAction) return
    const list = listRef.current
    const cue = cueRefs.current.get(preferredCueId)
    if (!list || !cue) return
    const top = cue.offsetTop - list.clientHeight / 2 + cue.offsetHeight / 2
    list.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
  }, [preferredCueId, hasSelection, activeAction, showTranslation])

  const handleCueClick = (event, cue) => {
    if (
      event.target.closest('[data-subtitle-text]') ||
      isPointInsideSubtitleText(
        event.currentTarget,
        event.clientX,
        event.clientY,
      )
    ) {
      return
    }
    clearSubtitleSelection()
    setHasSelection(false)
    setSelectionCandidate(null)
    setActiveAction(null)
    onSeek(cue.startSeconds)
  }

  const dismissLookup = () => {
    clearSubtitleSelection()
    setHasSelection(false)
    setSelectionCandidate(null)
    setActiveAction(null)
  }

  const openLookup = () => {
    if (!selectionCandidate?.lookup) return
    setActiveAction({ type: 'lookup', data: selectionCandidate.lookup })
    setSelectionCandidate(null)
    setHasSelection(false)
    clearSubtitleSelection()
  }

  const openSentenceAnalysis = () => {
    if (!selectionCandidate?.sentence) return
    onSentenceAnalysisOpen?.()
    setActiveAction({ type: 'sentence', data: selectionCandidate.sentence })
    setSelectionCandidate(null)
    setHasSelection(false)
    clearSubtitleSelection()
  }

  const preventManualScroll = (event) => {
    event.preventDefault()
  }

  const ensureContextVideo = useCallback(
    ({ itemId, lookup, onStatus }) => {
      const key = JSON.stringify([
        sourceFingerprint,
        lookup.itemType,
        itemId,
        lookup.paragraph,
        lookup.selectionStart,
        lookup.selectionEnd,
      ])
      let job = contextJobsRef.current.get(key)
      if (!job) {
        job = createContextVideoJob({
          itemId,
          lookup,
          videoUrl,
          playbackSource,
          sourceFingerprint,
          onFailed: () => contextJobsRef.current.delete(key),
        })
        contextJobsRef.current.set(key, job)
      }
      job.subscribers.add(onStatus)
      onStatus(job.status)
      return {
        promise: job.promise,
        unsubscribe: () => job.subscribers.delete(onStatus),
      }
    },
    [playbackSource, sourceFingerprint, videoUrl],
  )

  const panelRect = panelRef.current?.getBoundingClientRect()
  const selectionActionPosition =
    selectionCandidate && panelRect
      ? positionSelectionAction(
          selectionCandidate.anchorRect,
          panelRect,
          Number(!!selectionCandidate.lookup) +
            Number(!!selectionCandidate.sentence),
        )
      : null
  const activePopoverPosition =
    activeAction?.type === 'lookup' && panelRect
      ? positionPopover(
          activeAction.data.anchorRect,
          panelRect,
          { preferredWidth: 360, preferredHeight: 420 },
        )
      : null

  return (
    <section ref={panelRef} className="subtitle-panel" aria-label="字幕">
      <ol
        ref={listRef}
        className="subtitle-panel__list"
        onWheel={preventManualScroll}
        onTouchMove={preventManualScroll}
      >
        {visibleCues.map((cue) => {
          const active = activeIds.has(cue.id)
          return (
            <li
              key={cue.id}
              ref={(node) => {
                if (node) cueRefs.current.set(cue.id, node)
                else cueRefs.current.delete(cue.id)
              }}
              className={`subtitle-cue${active ? ' subtitle-cue--active' : ''}`}
              data-cue-id={cue.id}
              data-start={cue.startSeconds}
              data-end={cue.endSeconds}
              onClick={(event) => handleCueClick(event, cue)}
            >
              {cue.primaryLines.length > 0 && (
                <span
                  className="subtitle-cue__primary"
                  data-subtitle-text="primary"
                >
                  {cue.primaryLines.map((line, index) => (
                    <span
                      key={`${index}-${line}`}
                      data-subtitle-line="primary"
                      data-cue-id={cue.id}
                      data-line-index={index}
                    >
                      {line}
                    </span>
                  ))}
                </span>
              )}
              {showTranslation && cue.translationLines.length > 0 && (
                <span
                  className="subtitle-cue__translation"
                  data-subtitle-text="translation"
                >
                  {cue.translationLines.map((line, index) => (
                    <span key={`${index}-${line}`}>{line}</span>
                  ))}
                </span>
              )}
              {showTranslation && cue.annotationLines?.length > 0 && (
                <span
                  className="subtitle-cue__annotation"
                  data-subtitle-text="annotation"
                >
                  {cue.annotationLines.map((line, index) => (
                    <span key={`${index}-${line}`}>{line}</span>
                  ))}
                </span>
              )}
            </li>
          )
        })}
      </ol>
      {selectionCandidate && selectionActionPosition && (
        <div
          ref={selectionActionRef}
          className="subtitle-selection-actions"
          style={selectionActionPosition}
        >
          {selectionCandidate.lookup && (
            <button type="button" onClick={openLookup}>
              <Search size={15} aria-hidden="true" />
              查词
            </button>
          )}
          {selectionCandidate.sentence && (
            <button type="button" onClick={openSentenceAnalysis}>
              <ScanText size={15} aria-hidden="true" />
              分析
            </button>
          )}
        </div>
      )}
      {activeAction?.type === 'lookup' && activePopoverPosition && (
        <SubtitleLookupPopover
          lookup={activeAction.data}
          position={activePopoverPosition}
          onDismiss={dismissLookup}
          popoverRef={popoverRef}
          onSaveContext={ensureContextVideo}
        />
      )}
      {activeAction?.type === 'sentence' && (
        <SubtitleSentenceAnalysisPopover
          sentence={activeAction.data.sentence}
          onDismiss={dismissLookup}
          popoverRef={popoverRef}
        />
      )}
    </section>
  )
}

function createContextVideoJob({
  itemId,
  lookup,
  videoUrl,
  playbackSource,
  sourceFingerprint,
  onFailed,
}) {
  const controller = new AbortController()
  const job = {
    controller,
    subscribers: new Set(),
    status: { phase: 'generating', message: '正在生成语境' },
    promise: null,
  }
  const emit = (status) => {
    job.status = status
    job.subscribers.forEach((subscriber) => subscriber(status))
  }

  job.promise = (async () => {
    let textSaved = false
    try {
      const task = await createContextTask(
        {
          itemType: lookup.itemType,
          itemId,
          paragraph: lookup.paragraph,
          selectionStart: lookup.selectionStart,
          selectionEnd: lookup.selectionEnd,
          source: 'web_video',
        },
        { signal: controller.signal },
      )
      textSaved = true
      if (
        !task?.contextId ||
        !Number.isInteger(task.sentenceStart) ||
        !Number.isInteger(task.sentenceEnd)
      ) {
        throw new Error('后端未返回完整句子范围')
      }

      const clip = buildContextClip({
        sentenceStart: task.sentenceStart,
        sentenceEnd: task.sentenceEnd,
        focusStart: lookup.selectionStart,
        focusEnd: lookup.selectionEnd,
        cueRanges: lookup.cueRanges,
      })
      emit({
        phase: 'recording',
        message: `正在录制视频 0/${Math.ceil(clip.durationSeconds)} 秒`,
      })
      const recording = await recordContextClip({
        videoUrl,
        playbackSource,
        startSeconds: clip.clipStartSeconds,
        endSeconds: clip.clipEndSeconds,
        signal: controller.signal,
        onProgress: (elapsed, total) => {
          emit({
            phase: 'recording',
            message: `正在录制视频 ${Math.ceil(elapsed)}/${Math.ceil(total)} 秒`,
          })
        },
      })

      emit({ phase: 'uploading', message: '正在上传视频' })
      const video = await uploadContextVideo(
        task.contextId,
        recording.blob,
        recording.audioBlob,
        {
          sourceFingerprint,
          clipStartMs: Math.round(clip.clipStartSeconds * 1000),
          clipEndMs: Math.round(clip.clipEndSeconds * 1000),
          durationMs: clip.durationMs,
          subtitles: clip.subtitles,
        },
        { signal: controller.signal },
      )
      emit({ phase: 'success', message: '视频语境已保存' })
      return { task, video }
    } catch (error) {
      if (error.name !== 'AbortError') {
        emit({
          phase: 'error',
          message: textSaved
            ? '文本语境已保存，视频保存失败'
            : '语境保存失败',
          detail: error.message,
        })
      }
      onFailed()
      throw error
    }
  })()
  job.promise.catch(() => {})
  return job
}

function positionSelectionAction(anchor, panel, actionCount) {
  const width = actionCount * 72 + Math.max(0, actionCount - 1) * 6
  const height = 34
  const gap = 7
  const margin = 8
  const preferredTop = anchor.top - panel.top - height - gap
  const fallbackTop = anchor.bottom - panel.top + gap

  return {
    left: clamp(
      (anchor.left + anchor.right) / 2 - panel.left - width / 2,
      margin,
      panel.width - width - margin,
    ),
    top: clamp(
      preferredTop >= margin ? preferredTop : fallbackTop,
      margin,
      panel.height - height - margin,
    ),
  }
}

function positionPopover(
  anchor,
  panel,
  { preferredWidth, preferredHeight },
) {
  const margin = 8
  const gap = 8
  const width = Math.min(preferredWidth, panel.width - margin * 2)
  const maxHeight = Math.min(preferredHeight, panel.height - margin * 2)
  const belowTop = anchor.bottom - panel.top + gap
  const belowSpace = panel.height - margin - belowTop
  const aboveSpace = anchor.top - panel.top - gap - margin
  const useBelow = belowSpace >= Math.min(260, maxHeight) || belowSpace >= aboveSpace
  const availableHeight = Math.max(
    180,
    Math.min(maxHeight, useBelow ? belowSpace : aboveSpace),
  )
  const top = useBelow
    ? clamp(belowTop, margin, panel.height - availableHeight - margin)
    : clamp(
        anchor.top - panel.top - gap - availableHeight,
        margin,
        panel.height - availableHeight - margin,
      )

  return {
    left: clamp(
      (anchor.left + anchor.right) / 2 - panel.left - width / 2,
      margin,
      panel.width - width - margin,
    ),
    top,
    width,
    maxHeight: availableHeight,
  }
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), Math.max(min, max))
}
