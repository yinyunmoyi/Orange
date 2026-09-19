import { useEffect, useMemo, useRef, useState } from 'react'
import { analyzeSentence, translateSentenceStream } from '../api/word.js'
import './SentencePage.css'

// chunk.type → 中文名 + CSS 后缀（颜色定义在 CSS 里）
const TYPE_META = {
  main: { label: '主干', cls: 'main' },
  modifier: { label: '修饰', cls: 'modifier' },
  adverbial: { label: '状语', cls: 'adverbial' },
  parenthetical: { label: '插入语', cls: 'parenthetical' },
  participle: { label: '非谓语', cls: 'participle' },
  quote: { label: '引语', cls: 'quote' },
  other: { label: '其它', cls: 'other' },
}

function typeMeta(t) {
  return TYPE_META[t] || TYPE_META.other
}

function Dots() {
  return (
    <span className="dots-loader" aria-hidden="true">
      <span />
      <span />
      <span />
    </span>
  )
}

export default function SentencePage() {
  const [sentence, setSentence] = useState('')
  const [submittedSentence, setSubmittedSentence] = useState('')

  // 翻译流
  const [translation, setTranslation] = useState('')
  const [translationDone, setTranslationDone] = useState(false)
  const [translationError, setTranslationError] = useState('')

  // 语法分析
  const [analysis, setAnalysis] = useState(null)
  const [analysisDone, setAnalysisDone] = useState(false)
  const [analysisError, setAnalysisError] = useState('')

  const streamRef = useRef(null)
  const analyzeAbortRef = useRef(null)

  const running = !!submittedSentence && (!translationDone || !analysisDone)
  const view = submittedSentence ? 'result' : 'input'

  const hasChunks = !!(
    analysis &&
    Array.isArray(analysis.chunks) &&
    analysis.chunks.length > 0
  )

  const legendTypes = useMemo(() => {
    if (!hasChunks) return []
    const seen = new Set()
    for (const c of analysis.chunks) {
      const key = TYPE_META[c.type] ? c.type : 'other'
      seen.add(key)
    }
    return Array.from(seen)
  }, [analysis, hasChunks])

  const canSubmit = sentence.trim().length > 0 && !running

  useEffect(() => {
    return () => {
      // 组件卸载:中止残余的流和请求
      if (streamRef.current) streamRef.current.cancel()
      if (analyzeAbortRef.current) analyzeAbortRef.current.abort()
    }
  }, [])

  function handleAnalyze() {
    const trimmed = sentence.trim()
    if (!trimmed || running) return

    // 清空旧状态
    setSubmittedSentence(trimmed)
    setTranslation('')
    setTranslationDone(false)
    setTranslationError('')
    setAnalysis(null)
    setAnalysisDone(false)
    setAnalysisError('')

    // 1) 启动翻译流
    if (streamRef.current) streamRef.current.cancel()
    streamRef.current = translateSentenceStream(trimmed, {
      onDelta: (delta) => setTranslation((prev) => prev + delta),
      onDone: () => setTranslationDone(true),
      onError: (msg) => {
        setTranslationError(msg || '翻译失败')
        setTranslationDone(true)
      },
    })

    // 2) 启动语法分析
    if (analyzeAbortRef.current) analyzeAbortRef.current.abort()
    const ctl = new AbortController()
    analyzeAbortRef.current = ctl
    ;(async () => {
      try {
        const data = await analyzeSentence(trimmed, { signal: ctl.signal })
        if (ctl.signal.aborted) return
        setAnalysis(data)
      } catch (err) {
        if (ctl.signal.aborted) return
        setAnalysisError(err.message || '分析失败')
      } finally {
        if (!ctl.signal.aborted) setAnalysisDone(true)
      }
    })()
  }

  function handleReset() {
    if (streamRef.current) streamRef.current.cancel()
    if (analyzeAbortRef.current) analyzeAbortRef.current.abort()
    setSubmittedSentence('')
    setTranslation('')
    setTranslationDone(false)
    setTranslationError('')
    setAnalysis(null)
    setAnalysisDone(false)
    setAnalysisError('')
  }

  function handleKeyDown(e) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      if (canSubmit) handleAnalyze()
    }
  }

  const showAll = view === 'result'
  const heroKey = submittedSentence + (hasChunks ? '-colored' : '-plain')

  return (
    <main className="sentence-page">
      <header className="sentence-page__header">
        <h1 className="sentence-page__title">长难句</h1>
        {view === 'input' && (
          <p className="sentence-page__subtitle">
            粘贴一句英文,点击「分析」,查看翻译、结构和逐块解读
          </p>
        )}
      </header>

      <section
        className={`sentence-search ${view === 'input' ? '' : 'sentence-search--hidden'}`}
        aria-hidden={view !== 'input'}
      >
        <textarea
          className="sentence-search__input"
          placeholder="在这里粘贴一句英文…(Cmd/Ctrl + Enter 快速分析)"
          value={sentence}
          onChange={(e) => setSentence(e.target.value)}
          onKeyDown={handleKeyDown}
          rows={3}
          autoFocus
          disabled={view !== 'input'}
        />
        <button
          type="button"
          className="sentence-search__button"
          onClick={handleAnalyze}
          disabled={!canSubmit || view !== 'input'}
        >
          分析
        </button>
      </section>

      {showAll && (
        <section
          key={heroKey}
          className={`sentence-hero ${!analysisDone ? 'sentence-hero--analyzing' : ''}`}
        >
          <div className="sentence-hero__text">
            {hasChunks
              ? analysis.chunks.map((c, idx) => {
                  const meta = typeMeta(c.type)
                  const tip = c.role
                    ? `${meta.label} · ${c.role}${c.translation ? ' · ' + c.translation : ''}`
                    : `${meta.label}${c.translation ? ' · ' + c.translation : ''}`
                  return (
                    <span
                      key={idx}
                      className={`token token--${meta.cls} token--stagger`}
                      style={{ animationDelay: `${idx * 45}ms` }}
                      title={tip}
                      tabIndex={0}
                    >
                      {c.text}
                    </span>
                  )
                })
              : submittedSentence}
          </div>

          {hasChunks && legendTypes.length > 0 && (
            <ul className="sentence-hero__legend">
              {legendTypes.map((t, idx) => {
                const meta = typeMeta(t)
                return (
                  <li
                    key={t}
                    className="sentence-hero__legend-item"
                    style={{ animationDelay: `${300 + idx * 60}ms` }}
                  >
                    <span
                      className={`sentence-hero__swatch token--${meta.cls}`}
                    />
                    <span>{meta.label}</span>
                  </li>
                )
              })}
            </ul>
          )}

          {!running && (
            <button
              type="button"
              className="sentence-hero__reset"
              onClick={handleReset}
            >
              换一句
            </button>
          )}
        </section>
      )}

      {showAll && (
        <section className="sentence-result">
          {/* 汉语意思 */}
          <div className="sentence-result__block">
            <div className="sentence-result__header">
              <h2 className="sentence-result__section-title">汉语意思</h2>
              {!translationDone && <Dots />}
            </div>
            {translationError ? (
              <p className="sentence-error">分析失败:{translationError}</p>
            ) : (
              <p className="sentence-result__translation">
                {translation || (translationDone ? '（无内容）' : '')}
                {!translationDone && translation && (
                  <span className="cursor-blink" />
                )}
              </p>
            )}
          </div>

          {/* 语法分析 */}
          <div className="sentence-result__block">
            <div className="sentence-result__header">
              <h2 className="sentence-result__section-title">语法分析</h2>
              {!analysisDone && <Dots />}
            </div>
            {analysisError ? (
              <p className="sentence-error">分析失败:{analysisError}</p>
            ) : !analysisDone ? (
              <p className="sentence-result__placeholder">
                正在分析句子结构与语法块…
              </p>
            ) : (
              <>
                {analysis?.structure && (
                  <p className="sentence-structure">{analysis.structure}</p>
                )}
                {hasChunks && (
                  <div className="chunk-breakdown">
                    <div className="chunk-breakdown__row chunk-breakdown__row--head">
                      <span className="chunk-breakdown__idx">#</span>
                      <span className="chunk-breakdown__type">类型</span>
                      <span className="chunk-breakdown__text">原文</span>
                      <span className="chunk-breakdown__role">作用</span>
                      <span className="chunk-breakdown__trans">中文</span>
                    </div>
                    {analysis.chunks.map((c, idx) => {
                      const meta = typeMeta(c.type)
                      return (
                        <div key={idx} className="chunk-breakdown__row">
                          <span className="chunk-breakdown__idx">{idx + 1}</span>
                          <span className="chunk-breakdown__type">
                            <span
                              className={`chunk-breakdown__badge token--${meta.cls}`}
                            >
                              {meta.label}
                            </span>
                          </span>
                          <span className="chunk-breakdown__text">{c.text}</span>
                          <span className="chunk-breakdown__role">
                            {c.role || '-'}
                          </span>
                          <span className="chunk-breakdown__trans">
                            {c.translation || '-'}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                )}
              </>
            )}
          </div>
        </section>
      )}
    </main>
  )
}
