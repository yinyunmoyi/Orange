import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Star, X } from 'lucide-react'
import {
  analyzeSentence,
  favoriteSentence,
  getSentenceFavoriteStatus,
  translateSentenceStream,
} from '../../../api/word.js'

const TYPE_META = {
  main: { label: '主干', className: 'main' },
  modifier: { label: '修饰', className: 'modifier' },
  adverbial: { label: '状语', className: 'adverbial' },
  parenthetical: { label: '插入语', className: 'parenthetical' },
  participle: { label: '分词', className: 'participle' },
  quote: { label: '引语', className: 'quote' },
  other: { label: '其它', className: 'other' },
}

const loadingState = () => ({ status: 'loading', text: '', error: '' })

export default function SubtitleSentenceAnalysisPopover({
  sentence,
  popoverRef,
  onDismiss,
}) {
  const favoriteControllerRef = useRef(null)
  const [translation, setTranslation] = useState(loadingState)
  const [analysis, setAnalysis] = useState({
    status: 'loading',
    data: null,
    error: '',
  })
  const [favorite, setFavorite] = useState({
    status: 'loading',
    favorited: false,
    error: '',
  })
  const [favoriting, setFavoriting] = useState(false)
  const [favoriteError, setFavoriteError] = useState('')

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()
    const favoriteStatusController = new AbortController()
    setTranslation(loadingState())
    setAnalysis({ status: 'loading', data: null, error: '' })
    setFavorite({ status: 'loading', favorited: false, error: '' })
    setFavoriting(false)
    setFavoriteError('')
    favoriteControllerRef.current?.abort()
    favoriteControllerRef.current = null

    getSentenceFavoriteStatus(sentence, {
      signal: favoriteStatusController.signal,
    })
      .then((data) => {
        if (!cancelled) {
          setFavorite({
            status: 'success',
            favorited: data?.favorited === true,
            error: '',
          })
        }
      })
      .catch((error) => {
        if (!cancelled && error.name !== 'AbortError') {
          setFavorite({
            status: 'error',
            favorited: false,
            error: '收藏状态加载失败',
          })
        }
      })

    let translationRequest = null
    const startTimer = window.setTimeout(() => {
      if (cancelled) return
      translationRequest = translateSentenceStream(sentence, {
        onDelta: (delta) => {
          if (cancelled) return
          setTranslation((current) => ({
            status: 'streaming',
            text: current.text + delta,
            error: '',
          }))
        },
        onDone: () => {
          if (cancelled) return
          setTranslation((current) => ({
            status: current.text ? 'success' : 'error',
            text: current.text,
            error: current.text ? '' : '加载失败',
          }))
        },
        onError: () => {
          if (!cancelled) {
            setTranslation({ status: 'error', text: '', error: '加载失败' })
          }
        },
      })

      ;(async () => {
        try {
          const data = await analyzeSentence(sentence, {
            signal: controller.signal,
          })
          if (!cancelled) {
            setAnalysis({ status: 'success', data, error: '' })
          }
        } catch (error) {
          if (!cancelled && error.name !== 'AbortError') {
            setAnalysis({ status: 'error', data: null, error: '加载失败' })
          }
        }
      })()
    }, 0)

    return () => {
      cancelled = true
      window.clearTimeout(startTimer)
      controller.abort()
      favoriteStatusController.abort()
      favoriteControllerRef.current?.abort()
      favoriteControllerRef.current = null
      translationRequest?.cancel()
    }
  }, [sentence])

  const sentenceParts = useMemo(
    () => buildSentenceParts(sentence, analysis.data?.chunks),
    [analysis.data?.chunks, sentence],
  )

  useEffect(() => {
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previousOverflow
    }
  }, [])

  const canFavorite =
    translation.status === 'success' &&
    !!translation.text.trim() &&
    favorite.status !== 'loading' &&
    !favorite.favorited &&
    !favoriting

  const handleFavorite = async () => {
    if (!canFavorite) return
    const controller = new AbortController()
    favoriteControllerRef.current?.abort()
    favoriteControllerRef.current = controller
    setFavoriting(true)
    setFavoriteError('')
    try {
      await favoriteSentence(
        { sentence, translation: translation.text },
        { signal: controller.signal },
      )
      if (!controller.signal.aborted) {
        setFavorite({ status: 'success', favorited: true, error: '' })
      }
    } catch (error) {
      if (!controller.signal.aborted) {
        setFavoriteError(error.message || '收藏失败，请重试')
      }
    } finally {
      if (favoriteControllerRef.current === controller) {
        favoriteControllerRef.current = null
        if (!controller.signal.aborted) setFavoriting(false)
      }
    }
  }

  const favoriteLabel = favorite.favorited
    ? '已收藏'
    : favoriting
      ? '收藏中'
      : '收藏句子'

  return createPortal(
    <div
      className="subtitle-sentence-analysis-backdrop"
      role="presentation"
      onPointerDown={onDismiss}
    >
      <aside
        ref={popoverRef}
        className="subtitle-lookup subtitle-sentence-analysis"
        role="dialog"
        aria-modal="true"
        aria-label="句子分析"
        onPointerDown={(event) => event.stopPropagation()}
      >
        <header className="subtitle-lookup__header">
          <div className="subtitle-lookup__heading">
            <span className="subtitle-lookup__type">句子</span>
            <h2 className="subtitle-lookup__word">句子分析</h2>
          </div>
          <div className="subtitle-lookup__actions">
            <button
              type="button"
              className={`subtitle-lookup__icon-button${
                favorite.favorited ? ' is-active' : ''
              }`}
              disabled={!canFavorite}
              title={favoriteLabel}
              aria-label={favoriteLabel}
              aria-pressed={favorite.favorited}
              onClick={handleFavorite}
            >
              <Star
                size={19}
                fill={favorite.favorited ? 'currentColor' : 'none'}
                aria-hidden="true"
              />
            </button>
            <button
              type="button"
              className="subtitle-lookup__icon-button"
              title="关闭"
              aria-label="关闭句子分析"
              onClick={onDismiss}
            >
              <X size={19} aria-hidden="true" />
            </button>
          </div>
        </header>

        {(favorite.error || favoriteError) && (
          <p className="subtitle-lookup__action-error" role="alert">
            {favoriteError || favorite.error}
          </p>
        )}

        <div className="subtitle-sentence-analysis__original">
          {sentenceParts.map((part, index) => (
            <span
              key={`${part.text}-${index}`}
              className={
                part.type
                  ? `subtitle-sentence-analysis__token is-${typeMeta(part.type).className}`
                  : undefined
              }
            >
              {part.text}
            </span>
          ))}
        </div>

        <div className="subtitle-lookup__body">
          <section className="subtitle-lookup__section">
            <SectionHeader title="翻译" loading={translation.status === 'loading'} />
            <TranslationContent state={translation} />
          </section>

          <section className="subtitle-lookup__section">
            <SectionHeader title="语法解析" loading={analysis.status === 'loading'} />
            <AnalysisContent state={analysis} />
          </section>
        </div>
      </aside>
    </div>,
    document.body,
  )
}

function SectionHeader({ title, loading }) {
  return (
    <div className="subtitle-sentence-analysis__section-header">
      <h3 className="subtitle-lookup__section-title">{title}</h3>
      {loading && <LoadingDots />}
    </div>
  )
}

function TranslationContent({ state }) {
  if (state.status === 'error') {
    return <p className="subtitle-lookup__message is-error">加载失败</p>
  }
  if (state.status === 'loading') {
    return <p className="subtitle-lookup__message">正在翻译...</p>
  }
  return (
    <p className="subtitle-sentence-analysis__translation">
      {state.text}
      {state.status === 'streaming' && (
        <span className="subtitle-sentence-analysis__cursor" />
      )}
    </p>
  )
}

function AnalysisContent({ state }) {
  if (state.status === 'error') {
    return <p className="subtitle-lookup__message is-error">加载失败</p>
  }
  if (state.status === 'loading') {
    return <p className="subtitle-lookup__message">正在分析句子结构...</p>
  }

  const chunks = state.data?.chunks || []
  const structure = state.data?.structure?.trim()
  if (!structure && chunks.length === 0) {
    return <p className="subtitle-lookup__message">暂无内容</p>
  }

  return (
    <div className="subtitle-sentence-analysis__grammar">
      {structure && (
        <p className="subtitle-sentence-analysis__structure">{structure}</p>
      )}
      {chunks.map((chunk, index) => {
        const meta = typeMeta(chunk.type)
        return (
          <div
            key={`${chunk.text}-${index}`}
            className="subtitle-sentence-analysis__chunk"
          >
            <div className="subtitle-sentence-analysis__chunk-header">
              <span
                className={`subtitle-sentence-analysis__badge is-${meta.className}`}
              >
                {meta.label}
              </span>
              {chunk.role && (
                <span className="subtitle-sentence-analysis__role">
                  {chunk.role}
                </span>
              )}
            </div>
            <p
              className={`subtitle-sentence-analysis__chunk-text is-${meta.className}`}
            >
              {chunk.text}
            </p>
            {chunk.translation && (
              <p className="subtitle-sentence-analysis__chunk-translation">
                {chunk.translation}
              </p>
            )}
          </div>
        )
      })}
    </div>
  )
}

function LoadingDots() {
  return (
    <span className="subtitle-sentence-analysis__dots" aria-label="加载中">
      <span />
      <span />
      <span />
    </span>
  )
}

function typeMeta(type) {
  return TYPE_META[type] || TYPE_META.other
}

function buildSentenceParts(sentence, chunks = []) {
  if (!Array.isArray(chunks) || chunks.length === 0) {
    return [{ text: sentence, type: '' }]
  }
  if (chunks.map((chunk) => chunk.text).join('') === sentence) {
    return chunks.map((chunk) => ({ text: chunk.text, type: chunk.type }))
  }

  const parts = []
  let cursor = 0
  for (const chunk of chunks) {
    if (!chunk?.text) continue
    const index = sentence.indexOf(chunk.text, cursor)
    if (index < 0) break
    if (index > cursor) {
      parts.push({ text: sentence.slice(cursor, index), type: '' })
    }
    parts.push({ text: chunk.text, type: chunk.type })
    cursor = index + chunk.text.length
  }
  if (cursor < sentence.length) {
    parts.push({ text: sentence.slice(cursor), type: '' })
  }
  return parts.length ? parts : [{ text: sentence, type: '' }]
}
