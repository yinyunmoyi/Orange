import { useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import '../WordCard.css'
import wordData from '../data/wordData.js'
import {
  createActionEventId,
  lookupWord,
  getMeaning,
  getContexts,
  favoriteWord,
  getFavoriteStatus,
  recordFavoriteAction,
} from '../api/word.js'
import WordMetaChips from '../components/WordMetaChips.jsx'

// 校验路径上的单词参数，非法或缺失时回退到默认词
function resolveWord(raw) {
  const decoded = raw ? decodeURIComponent(raw).toLowerCase() : ''
  return /^[a-z]+$/.test(decoded) ? decoded : wordData.word
}

// 将句子中的目标词高亮显示
function HighlightedSentence({ sentence, highlight }) {
  if (!highlight) return <>{sentence}</>

  const parts = sentence.split(new RegExp(`(${highlight})`, 'gi'))

  return (
    <>
      {parts.map((part, index) =>
        part.toLowerCase() === highlight.toLowerCase() ? (
          <mark key={index} className="highlight">
            {part}
          </mark>
        ) : (
          <span key={index}>{part}</span>
        )
      )}
    </>
  )
}

// 加载中的跳动省略号
function LoadingDots() {
  return (
    <span className="loading-dots" aria-label="加载中">
      <span className="loading-dots__dot" />
      <span className="loading-dots__dot" />
      <span className="loading-dots__dot" />
    </span>
  )
}

// 发音按钮组：每个口音一个按钮，点击播放对应音频
function Pronunciations({ pronunciations, itemId }) {
  const audioRef = useRef(null)
  const [playing, setPlaying] = useState(null)

  if (!pronunciations || pronunciations.length === 0) return null

  const play = (item) => {
    const audio = audioRef.current
    if (!audio) return
    audio.src = item.audioUrl
    setPlaying(item.accent)
    audio
      .play()
      .then(() => {
        if (Number.isInteger(itemId) && itemId > 0) {
          recordFavoriteAction({
            itemType: 'word',
            itemId,
            eventId: createActionEventId(),
            action: 'audio_played',
            source: 'web_word_detail',
            metadata: { accent: item.accent || '' },
          }).catch(() => {})
        }
      })
      .catch(() => setPlaying(null))
  }

  return (
    <div className="pronunciations">
      {pronunciations.map((item) => (
        <button
          type="button"
          className={`pronounce-btn${playing === item.accent ? ' is-playing' : ''}`}
          key={item.accent + item.audioUrl}
          onClick={() => play(item)}
        >
          <span className="pronounce-btn__icon" aria-hidden="true">
            🔊
          </span>
          {item.accent}
        </button>
      ))}
      {/* 共用一个隐藏 audio 元素承载流式音频 */}
      <audio ref={audioRef} onEnded={() => setPlaying(null)} hidden />
    </div>
  )
}

export default function WordDetailPage() {
  const { word: rawWord } = useParams()
  const word = useMemo(() => resolveWord(rawWord), [rawWord])
  const detailEventId = useMemo(
    () => ({ word, id: createActionEventId() }),
    [word],
  ).id

  const [info, setInfo] = useState(null)
  const [status, setStatus] = useState('loading')
  const [errorMsg, setErrorMsg] = useState('')

  const [meanings, setMeanings] = useState([])
  const [meaningStatus, setMeaningStatus] = useState('loading')
  const [contexts, setContexts] = useState([])

  const [favorited, setFavorited] = useState(false)
  const [favoriteItemId, setFavoriteItemId] = useState(0)
  const [favoriteBusy, setFavoriteBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    setStatus('loading')
    setMeanings([])
    setMeaningStatus('loading')
    setContexts([])
    setFavorited(false)
    setFavoriteItemId(0)

    lookupWord(word)
      .then((data) => {
        if (cancelled) return
        setInfo(data)
        setStatus('success')
      })
      .catch((err) => {
        if (cancelled) return
        setErrorMsg(err.message || '加载失败')
        setStatus('error')
      })

    getMeaning(word)
      .then((data) => {
        if (cancelled) return
        setMeanings(data.meanings || [])
        setMeaningStatus('success')
      })
      .catch(() => {
        if (!cancelled) setMeaningStatus('error')
      })

    getContexts(word)
      .then((data) => {
        if (!cancelled) setContexts(data.contexts || [])
      })
      .catch(() => {})

    getFavoriteStatus(word)
      .then((data) => {
        if (cancelled) return
        setFavorited(!!data.favorited)
        setFavoriteItemId(data.itemId || 0)
        if (data.favorited && Number.isInteger(data.itemId) && data.itemId > 0) {
          recordFavoriteAction({
            itemType: 'word',
            itemId: data.itemId,
            eventId: detailEventId,
            action: 'detail_opened',
            source: 'web_word_detail',
          }).catch(() => {})
        }
      })
      .catch(() => {})

    return () => {
      cancelled = true
    }
  }, [detailEventId, word])

  const handleFavorite = async () => {
    if (favorited || favoriteBusy) return
    if (status !== 'success' || !info) {
      alert('页面数据还没加载完，请稍后再收藏')
      return
    }
    setFavoriteBusy(true)
    try {
      const data = await favoriteWord({
        word,
        wordData: info,
        meaning: meaningStatus === 'success' ? { word, meanings } : null,
        contexts: contexts.length > 0 ? { word, contexts } : null,
      })
      setFavorited(true)
      setFavoriteItemId(data.itemId || 0)
    } catch (err) {
      alert(err.message || '收藏失败')
    } finally {
      setFavoriteBusy(false)
    }
  }

  return (
    <main className="word-page">
      <header className="word-hero">
        <div className="word-hero__title-row">
          <h1 className="word-hero__word">{word}</h1>
          <button
            type="button"
            className={`favorite-btn${favorited ? ' is-active' : ''}`}
            disabled={favorited || favoriteBusy}
            onClick={handleFavorite}
            title={favorited ? '已收藏' : '收藏该单词'}
            aria-label={favorited ? '已收藏' : '收藏该单词'}
          >
            {favorited ? '★' : '☆'}
          </button>
        </div>

        {status === 'loading' && (
          <p className="word-hero__phonetic word-hero__hint">加载中…</p>
        )}
        {status === 'error' && (
          <p className="word-hero__phonetic word-hero__hint word-hero__hint--error">
            英文信息加载失败：{errorMsg}
          </p>
        )}
        {status === 'success' && (
          <>
            {info.phonetic && (
              <p className="word-hero__phonetic">{info.phonetic}</p>
            )}
            <Pronunciations
              pronunciations={info.pronunciations}
              itemId={favoriteItemId}
            />
            <WordMetaChips
              pos={info.pos}
              oxford={info.oxford}
              tags={info.tags}
            />
          </>
        )}

        {meaningStatus === 'loading' && (
          <p className="word-hero__meaning word-hero__meaning--loading">
            <LoadingDots />
          </p>
        )}
        {meaningStatus === 'success' && meanings.length > 0 && (
          <ul className="word-hero__meanings">
            {meanings.map((m, i) => (
              <li className="cn-meaning" key={i}>
                {m.partOfSpeech && (
                  <span className="cn-meaning__pos">{m.partOfSpeech}</span>
                )}
                <span className="cn-meaning__text">{m.meaning}</span>
              </li>
            ))}
          </ul>
        )}
      </header>

      {contexts.length > 0 && (
        <section className="card-section">
          <h2 className="card-section__title">知识网络</h2>
          {contexts.map((context, index) => (
            <div className="context-item" key={context.id}>
              <span className="context-item__label">语境 {index + 1}</span>
              <p className="context-item__sentence">
                <HighlightedSentence
                  sentence={context.sentence}
                  highlight={context.highlight}
                />
              </p>
            </div>
          ))}
        </section>
      )}

      <section className="card-section">
        <h2 className="card-section__title">英英释义</h2>

        {status === 'loading' && (
          <p className="card-section__hint">加载中…</p>
        )}
        {status === 'error' && (
          <p className="card-section__hint card-section__hint--error">
            {errorMsg}
          </p>
        )}
        {status === 'success' &&
          info.meanings.map((def, defIndex) => (
            <div className="definition-block" key={defIndex}>
              <p className="definition-block__pos">
                {word} {def.partOfSpeech}
              </p>
              <ol className="sense-list">
                {def.definitions.map((sense, senseIndex) => (
                  <li className="sense-item" key={senseIndex}>
                    <p className="sense-item__definition">{sense.definition}</p>
                    {sense.example && (
                      <p className="sense-item__example">“{sense.example}”</p>
                    )}
                    {sense.synonyms?.length > 0 && (
                      <p className="sense-item__synonyms">
                        <span className="sense-item__synonyms-label">
                          synonymous:
                        </span>
                        {sense.synonyms.map((syn) => (
                          <span className="synonym-tag" key={syn}>
                            {syn}
                          </span>
                        ))}
                      </p>
                    )}
                  </li>
                ))}
              </ol>
            </div>
          ))}
      </section>
    </main>
  )
}
