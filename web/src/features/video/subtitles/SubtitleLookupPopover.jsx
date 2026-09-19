import { useEffect, useMemo, useRef, useState } from 'react'
import { Hourglass, Pencil, Star, Volume2, X } from 'lucide-react'
import {
  createActionEventId,
  explainSelection,
  favoritePhrase,
  favoriteWord,
  getFavoriteStatus,
  getMeaning,
  getNote,
  getPhraseFavoriteStatus,
  lookupPhrase,
  lookupWord,
  recordFavoriteAction,
  resetFavoriteLearning,
  saveNote,
} from '../../../api/word.js'
import WordMetaChips from '../../../components/WordMetaChips.jsx'
import {
  MAX_NOTE_CODE_POINTS,
  canSaveNote,
  noteCodePointCount,
} from './noteModel.js'

const loadingState = () => ({ status: 'loading', data: null, error: '' })

export default function SubtitleLookupPopover({
  lookup,
  position,
  onDismiss,
  popoverRef,
  onSaveContext,
}) {
  const isPhrase = lookup.itemType === 'phrase'
  const lookupEventKey = `${lookup.itemType}\u0000${lookup.text}\u0000${lookup.context}\u0000${lookup.wordStart}\u0000${lookup.wordEnd}`
  const lookupEventId = useMemo(
    () => ({ key: lookupEventKey, id: createActionEventId() }),
    [lookupEventKey],
  ).id
  const audioRef = useRef(null)
  const resetControllerRef = useRef(null)
  const noteSaveControllerRef = useRef(null)
  const [english, setEnglish] = useState(loadingState)
  const [chinese, setChinese] = useState(loadingState)
  const [explanation, setExplanation] = useState(loadingState)
  const [note, setNote] = useState(loadingState)
  const [noteEditing, setNoteEditing] = useState(false)
  const [noteDraft, setNoteDraft] = useState('')
  const [noteSaving, setNoteSaving] = useState(false)
  const [noteSaveError, setNoteSaveError] = useState('')
  const [favorite, setFavorite] = useState(loadingState)
  const [initiallyFavorited, setInitiallyFavorited] = useState(null)
  const [favoriting, setFavoriting] = useState(false)
  const [resetting, setResetting] = useState(false)
  const [favoriteError, setFavoriteError] = useState('')
  const [playing, setPlaying] = useState('')
  const [audioError, setAudioError] = useState('')
  const [contextStatus, setContextStatus] = useState(null)

  useEffect(() => {
    let cancelled = false
    const audio = audioRef.current
    const noteLoadController = new AbortController()
    setEnglish(isPhrase ? { status: 'idle', data: null, error: '' } : loadingState())
    setChinese(loadingState())
    setExplanation(loadingState())
    setNote(loadingState())
    setNoteEditing(false)
    setNoteDraft('')
    setNoteSaving(false)
    setNoteSaveError('')
    setFavorite(loadingState())
    setInitiallyFavorited(null)
    setFavoriting(false)
    setResetting(false)
    setFavoriteError('')
    setPlaying('')
    setAudioError('')
    setContextStatus(null)
    resetControllerRef.current?.abort()
    resetControllerRef.current = null
    noteSaveControllerRef.current?.abort()
    noteSaveControllerRef.current = null

    const settle = (request, setter) => {
      request
        .then((data) => {
          if (!cancelled) setter({ status: 'success', data, error: '' })
        })
        .catch((error) => {
          if (!cancelled) {
            setter({
              status: 'error',
              data: null,
              error: error.message || '加载失败',
            })
          }
        })
    }

    if (!isPhrase) {
      settle(lookupWord(lookup.text), setEnglish)
    }
    settle(
      isPhrase ? lookupPhrase(lookup.text) : getMeaning(lookup.text),
      setChinese,
    )
    settle(
      explainSelection({
        word: lookup.text,
        context: lookup.context,
        wordStart: lookup.wordStart,
        wordEnd: lookup.wordEnd,
      }),
      setExplanation,
    )
    settle(
      getNote(lookup.itemType, lookup.text, {
        signal: noteLoadController.signal,
      }),
      setNote,
    )
    const favoriteRequest = isPhrase
      ? getPhraseFavoriteStatus(lookup.text)
      : getFavoriteStatus(lookup.text)
    favoriteRequest
      .then((data) => {
        if (cancelled) return
        setFavorite({ status: 'success', data, error: '' })
        setInitiallyFavorited(data?.favorited === true)
        if (
          data?.favorited === true &&
          Number.isInteger(data.itemId) &&
          data.itemId > 0
        ) {
          recordFavoriteAction({
            itemType: lookup.itemType,
            itemId: data.itemId,
            eventId: lookupEventId,
            action: 'lookup_opened',
            source: 'web_video',
          }).catch(() => {})
        }
      })
      .catch((error) => {
        if (cancelled) return
        setFavorite({
          status: 'error',
          data: null,
          error: error.message || '加载失败',
        })
        setInitiallyFavorited(false)
      })

    return () => {
      cancelled = true
      noteLoadController.abort()
      resetControllerRef.current?.abort()
      resetControllerRef.current = null
      noteSaveControllerRef.current?.abort()
      noteSaveControllerRef.current = null
      stopAudio(audio)
    }
  }, [
    isPhrase,
    lookup.context,
    lookup.itemType,
    lookupEventId,
    lookup.text,
    lookup.wordEnd,
    lookup.wordStart,
  ])

  useEffect(() => {
    const itemId = favorite.data?.itemId
    if (
      favorite.status !== 'success' ||
      !favorite.data?.favorited ||
      !Number.isInteger(itemId) ||
      itemId <= 0 ||
      !onSaveContext
    ) {
      return undefined
    }
    const job = onSaveContext({
      itemId,
      lookup,
      onStatus: setContextStatus,
    })
    return job.unsubscribe
  }, [favorite, lookup, onSaveContext])

  const canFavorite =
    favorite.status === 'success' &&
    !favorite.data?.favorited &&
    !favoriting &&
    (isPhrase
      ? chinese.status === 'success' &&
        (chinese.data?.meanings?.length || 0) > 0
      : english.status === 'success' && !!english.data)

  const handleFavorite = async () => {
    if (!canFavorite) return
    setFavoriting(true)
    setFavoriteError('')

    try {
      const data = isPhrase
        ? await favoritePhrase({
            phrase: lookup.text,
            meaning: chinese.data,
          })
        : await favoriteWord({
            word: lookup.text,
            wordData: english.data,
            meaning: chinese.status === 'success' ? chinese.data : null,
          })
      setFavorite({
        status: 'success',
        data: { favorited: true, itemId: data.itemId },
        error: '',
      })
    } catch (error) {
      setFavoriteError(error.message || '收藏失败')
    } finally {
      setFavoriting(false)
    }
  }

  const handlePlay = (pronunciation, index) => {
    const audio = audioRef.current
    if (!audio || !pronunciation.audioUrl) return

    const key = `${pronunciation.accent}-${index}`
    stopAudio(audio)
    setAudioError('')
    setPlaying(key)
    audio.src = pronunciation.audioUrl
    audio
      .play()
      .then(() => {
        const itemId = favorite.data?.itemId
        if (favorite.data?.favorited && Number.isInteger(itemId) && itemId > 0) {
          recordFavoriteAction({
            itemType: lookup.itemType,
            itemId,
            eventId: createActionEventId(),
            action: 'audio_played',
            source: 'web_video',
            metadata: { accent: pronunciation.accent || '' },
          }).catch(() => {})
        }
      })
      .catch(() => {
        setPlaying('')
        setAudioError('发音播放失败')
      })
  }

  const handleResetLearning = async () => {
    const itemId = favorite.data?.itemId
    if (
      resetting ||
      favorite.status !== 'success' ||
      favorite.data?.favorited !== true ||
      !Number.isInteger(itemId) ||
      itemId <= 0
    ) {
      return
    }

    const controller = new AbortController()
    resetControllerRef.current?.abort()
    resetControllerRef.current = controller
    setResetting(true)
    try {
      await resetFavoriteLearning(lookup.itemType, itemId, {
        signal: controller.signal,
      })
    } catch {
      // Match the client: keep the popup unchanged and re-enable the action.
    } finally {
      if (
        resetControllerRef.current === controller &&
        !controller.signal.aborted
      ) {
        resetControllerRef.current = null
        setResetting(false)
      }
    }
  }

  const handleEditNote = () => {
    setNoteDraft(note.status === 'success' ? note.data?.note || '' : '')
    setNoteSaveError('')
    setNoteEditing(true)
  }

  const handleCancelNote = () => {
    if (noteSaving) return
    setNoteEditing(false)
    setNoteSaveError('')
  }

  const handleSaveNote = async () => {
    if (!canSaveNote(noteDraft, noteSaving)) return

    const controller = new AbortController()
    noteSaveControllerRef.current?.abort()
    noteSaveControllerRef.current = controller
    setNoteSaving(true)
    setNoteSaveError('')
    try {
      const data = await saveNote(
        lookup.itemType,
        lookup.text,
        noteDraft,
        { signal: controller.signal },
      )
      if (controller.signal.aborted) return
      setNote({ status: 'success', data, error: '' })
      setNoteEditing(false)
    } catch {
      if (!controller.signal.aborted) {
        setNoteSaveError('保存失败，请重试')
      }
    } finally {
      if (
        noteSaveControllerRef.current === controller &&
        !controller.signal.aborted
      ) {
        noteSaveControllerRef.current = null
        setNoteSaving(false)
      }
    }
  }

  const favorited = favorite.status === 'success' && favorite.data?.favorited
  const favoriteLabel = favorited
    ? '已收藏'
    : favoriting
      ? '收藏中'
      : '收藏'

  return (
    <aside
      ref={popoverRef}
      className="subtitle-lookup"
      style={position}
      role="dialog"
      aria-label={`${lookup.text}查词结果`}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <header className="subtitle-lookup__header">
        <div className="subtitle-lookup__heading">
          <span className="subtitle-lookup__type">
            {isPhrase ? '短语' : '单词'}
          </span>
          <h2 className="subtitle-lookup__word">{lookup.text}</h2>
          {!isPhrase && (
            <WordPronunciationInline
              state={english}
              playing={playing}
              audioError={audioError}
              onPlay={handlePlay}
            />
          )}
        </div>
        <div className="subtitle-lookup__actions">
          <button
            type="button"
            className={`subtitle-lookup__icon-button${
              favorited ? ' is-active' : ''
            }`}
            disabled={!canFavorite}
            title={favoriteLabel}
            aria-label={favoriteLabel}
            aria-pressed={!!favorited}
            onClick={handleFavorite}
          >
            <Star
              size={19}
              fill={favorited ? 'currentColor' : 'none'}
              aria-hidden="true"
            />
          </button>
          {initiallyFavorited === true && (
            <button
              type="button"
              className="subtitle-lookup__icon-button subtitle-lookup__reset-button"
              disabled={resetting}
              title="重置学习进度"
              aria-label="重置学习进度"
              onClick={handleResetLearning}
            >
              <Hourglass size={19} aria-hidden="true" />
            </button>
          )}
          <button
            type="button"
            className="subtitle-lookup__icon-button"
            title="关闭"
            aria-label="关闭查词"
            onClick={onDismiss}
          >
            <X size={19} aria-hidden="true" />
          </button>
        </div>
      </header>

      {!isPhrase && <WordPronunciation state={english} />}

      {(favorite.status === 'error' || favoriteError) && (
        <p className="subtitle-lookup__action-error" role="alert">
          {favoriteError || '收藏状态加载失败'}
        </p>
      )}
      {contextStatus && (
        <p
          className={`subtitle-lookup__context-status is-${contextStatus.phase}`}
          role={contextStatus.phase === 'error' ? 'alert' : 'status'}
          title={contextStatus.detail || contextStatus.message}
        >
          {contextStatus.message}
        </p>
      )}

      <div className="subtitle-lookup__body">
        <LookupSection title="中文释义">
          <ChineseMeaning state={chinese} />
        </LookupSection>

        <LookupSection title="AI 语境解释">
          <Explanation state={explanation} />
        </LookupSection>

        {!isPhrase && (
          <LookupSection title="英文释义">
            <EnglishMeaning state={english} />
          </LookupSection>
        )}

        <NoteSection
          state={note}
          editing={noteEditing}
          draft={noteDraft}
          saving={noteSaving}
          saveError={noteSaveError}
          isPhrase={isPhrase}
          onEdit={handleEditNote}
          onDraftChange={setNoteDraft}
          onCancel={handleCancelNote}
          onSave={handleSaveNote}
        />
      </div>

      <audio
        ref={audioRef}
        hidden
        onEnded={() => setPlaying('')}
        onError={() => {
          setPlaying('')
          setAudioError('发音播放失败')
        }}
      />
    </aside>
  )
}

function WordPronunciationInline({ state, playing, audioError, onPlay }) {
  if (state.status === 'loading') {
    return (
      <span className="subtitle-lookup__phonetic-inline is-muted">加载中...</span>
    )
  }
  if (state.status !== 'success') return null

  const pronunciations = (state.data?.pronunciations || []).filter(
    (item) => item.audioUrl,
  )
  const phonetic = state.data?.phonetic

  return (
    <div className="subtitle-lookup__phonetic-inline">
      {phonetic && (
        <span className="subtitle-lookup__phonetic">{phonetic}</span>
      )}
      {pronunciations.length > 0 && (
        <div className="subtitle-lookup__audio-list">
          {pronunciations.map((item, index) => {
            const key = `${item.accent}-${index}`
            return (
              <button
                type="button"
                className={`subtitle-lookup__audio-button${
                  playing === key ? ' is-playing' : ''
                }`}
                key={`${item.audioUrl}-${index}`}
                title={`播放${item.accent || '发音'}`}
                aria-label={`播放${item.accent || '发音'}`}
                onClick={() => onPlay(item, index)}
              >
                <Volume2 size={16} aria-hidden="true" />
                {item.accent && <span>{item.accent}</span>}
              </button>
            )
          })}
        </div>
      )}
      {audioError && (
        <span className="subtitle-lookup__audio-error">{audioError}</span>
      )}
    </div>
  )
}

function WordPronunciation({ state }) {
  if (state.status !== 'success') return null
  const hasMeta =
    Number(state.data?.oxford) === 1 ||
    (Array.isArray(state.data?.tags) && state.data.tags.length > 0)
  if (!hasMeta) return null

  return (
    <div className="subtitle-lookup__meta-row">
      <WordMetaChips
        pos={state.data?.pos}
        oxford={state.data?.oxford}
        tags={state.data?.tags}
        showPos={false}
      />
    </div>
  )
}

function LookupSection({ title, children }) {
  return (
    <section className="subtitle-lookup__section">
      <h3 className="subtitle-lookup__section-title">{title}</h3>
      {children}
    </section>
  )
}

function NoteSection({
  state,
  editing,
  draft,
  saving,
  saveError,
  isPhrase,
  onEdit,
  onDraftChange,
  onCancel,
  onSave,
}) {
  const count = noteCodePointCount(draft)
  const overLimit = count > MAX_NOTE_CODE_POINTS

  return (
    <section className="subtitle-lookup__section subtitle-lookup__note">
      <div className="subtitle-lookup__note-heading">
        <h3 className="subtitle-lookup__section-title">备注</h3>
        {!editing && (
          <button
            type="button"
            className="subtitle-lookup__note-edit"
            disabled={state.status === 'loading'}
            title="编辑备注"
            aria-label="编辑备注"
            onClick={onEdit}
          >
            <Pencil size={16} aria-hidden="true" />
          </button>
        )}
      </div>

      {editing ? (
        <div className="subtitle-lookup__note-editor">
          <textarea
            value={draft}
            disabled={saving}
            placeholder={`记录关于这个${isPhrase ? '短语' : '单词'}的内容`}
            aria-label="备注"
            aria-invalid={overLimit}
            autoFocus
            onChange={(event) => onDraftChange(event.target.value)}
          />
          <div className="subtitle-lookup__note-footer">
            <span
              className={`subtitle-lookup__note-count${
                overLimit ? ' is-error' : ''
              }`}
            >
              {count}/{MAX_NOTE_CODE_POINTS}
            </span>
            <div className="subtitle-lookup__note-actions">
              <button
                type="button"
                className="subtitle-lookup__note-cancel"
                disabled={saving}
                onClick={onCancel}
              >
                取消
              </button>
              <button
                type="button"
                className="subtitle-lookup__note-save"
                disabled={!canSaveNote(draft, saving)}
                onClick={onSave}
              >
                {saving ? '保存中' : '完成'}
              </button>
            </div>
          </div>
          {saveError && (
            <p className="subtitle-lookup__note-error" role="alert">
              {saveError}
            </p>
          )}
        </div>
      ) : state.status === 'loading' ? (
        <LookupMessage text="加载中..." />
      ) : state.status === 'error' ? (
        <LookupMessage text="备注加载失败" error />
      ) : state.data?.note ? (
        <p className="subtitle-lookup__note-content">{state.data.note}</p>
      ) : (
        <LookupMessage text="暂无备注" />
      )}
    </section>
  )
}

function ChineseMeaning({ state }) {
  if (state.status === 'loading') return <LookupMessage text="加载中..." />
  if (state.status === 'error') return <LookupMessage text="加载失败" error />

  const meanings = state.data?.meanings || []
  if (meanings.length === 0) return <LookupMessage text="暂无释义" />

  return (
    <ul className="subtitle-lookup__meaning-list">
      {meanings.map((item, index) => (
        <li key={`${item.partOfSpeech}-${item.meaning}-${index}`}>
          {item.partOfSpeech && (
            <span className="subtitle-lookup__pos">{item.partOfSpeech}</span>
          )}
          <span>{item.meaning}</span>
        </li>
      ))}
    </ul>
  )
}

function Explanation({ state }) {
  if (state.status === 'loading') return <LookupMessage text="加载中..." />
  if (state.status === 'error') return <LookupMessage text="加载失败" error />
  if (!state.data?.explanation) return <LookupMessage text="暂无解释" />

  return <p className="subtitle-lookup__explanation">{state.data.explanation}</p>
}

function EnglishMeaning({ state }) {
  if (state.status === 'loading') return <LookupMessage text="加载中..." />
  if (state.status === 'error') return <LookupMessage text="加载失败" error />

  const meanings = state.data?.meanings || []
  if (meanings.length === 0) return <LookupMessage text="暂无释义" />

  return (
    <div className="subtitle-lookup__definitions">
      {meanings.map((group, groupIndex) => (
        <div key={`${group.partOfSpeech}-${groupIndex}`}>
          {group.partOfSpeech && (
            <span className="subtitle-lookup__pos">{group.partOfSpeech}</span>
          )}
          <ol>
            {(group.definitions || []).map((item, index) => (
              <li key={`${item.definition}-${index}`}>{item.definition}</li>
            ))}
          </ol>
        </div>
      ))}
    </div>
  )
}

function LookupMessage({ text, error = false }) {
  return (
    <p className={`subtitle-lookup__message${error ? ' is-error' : ''}`}>
      {text}
    </p>
  )
}

function stopAudio(audio) {
  if (!audio) return
  audio.pause()
  audio.removeAttribute('src')
  audio.load()
}
