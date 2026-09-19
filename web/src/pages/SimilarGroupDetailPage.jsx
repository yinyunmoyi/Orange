import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import '../WordCard.css'
import { getSimilarGroup } from '../api/word.js'

export default function SimilarGroupDetailPage() {
  const { id } = useParams()
  const [detail, setDetail] = useState(null)
  const [status, setStatus] = useState('loading')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    let cancelled = false
    setStatus('loading')
    getSimilarGroup(id)
      .then((data) => {
        if (cancelled) return
        setDetail(data)
        setStatus(data ? 'success' : 'missing')
      })
      .catch((err) => {
        if (cancelled) return
        setErrorMsg(err.message || '加载失败')
        setStatus('error')
      })
    return () => {
      cancelled = true
    }
  }, [id])

  return (
    <main className="similar-detail-page">
      {status === 'loading' && <p className="favorite-empty">加载中…</p>}
      {status === 'error' && (
        <p className="favorite-empty favorite-empty--error">加载失败:{errorMsg}</p>
      )}
      {status === 'missing' && (
        <p className="favorite-empty">该单词组不存在或已被合并</p>
      )}
      {status === 'success' && detail && (
        <>
          <header className="similar-detail-page__header">
            <h1 className="similar-detail-page__title">相似单词组</h1>
            <span className="similar-card__badge similar-card__badge--inline">
              共 {detail.memberCount} 个
            </span>
          </header>
          <div className="similar-member-grid">
            {detail.members.map((m) => (
              <Link
                key={m.word}
                to={`/console/word/${encodeURIComponent(m.word)}`}
                className="similar-member-card"
              >
                <span className="similar-member-card__word">{m.word}</span>
                {m.topMeaningText && (
                  <span className="similar-member-card__meaning">
                    {m.topMeaningPos && (
                      <span className="similar-member-card__pos">
                        {m.topMeaningPos}
                      </span>
                    )}
                    {m.topMeaningText}
                  </span>
                )}
              </Link>
            ))}
          </div>
        </>
      )}
    </main>
  )
}
