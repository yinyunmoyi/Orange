import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import '../WordCard.css'
import { listSimilarGroups } from '../api/word.js'

export default function SimilarGroupListPage() {
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(null)
  const [loading, setLoading] = useState(true)
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setErrorMsg('')
    listSimilarGroups()
      .then((data) => {
        if (cancelled) return
        setItems(data.items || [])
        setTotal(data.total ?? (data.items || []).length)
      })
      .catch((err) => {
        if (cancelled) return
        setErrorMsg(err.message || '加载失败')
        setItems([])
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <main className="similar-list-page">
      <header className="similar-list-page__header">
        <h1 className="similar-list-page__title">
          相似单词
          {total !== null && (
            <span className="page-title__count">共 {total} 个词组</span>
          )}
        </h1>
      </header>

      {loading && <p className="favorite-empty">加载中…</p>}
      {!loading && errorMsg && (
        <p className="favorite-empty favorite-empty--error">加载失败:{errorMsg}</p>
      )}
      {!loading && !errorMsg && items.length === 0 && (
        <p className="favorite-empty">还没有相似单词组</p>
      )}
      {!loading && items.length > 0 && (
        <div className="similar-grid">
          {items.map((item) => {
            const preview = item.preview || []
            const displayed = preview.slice(0, 3).join(' · ')
            const hasMore = item.memberCount > preview.length
            return (
              <Link
                key={item.id}
                to={`/console/same_group/${item.id}`}
                className={`similar-card similar-card--stack-${Math.min(item.memberCount, 3)}`}
              >
                <span className="similar-card__badge">共 {item.memberCount} 个</span>
                <span className="similar-card__preview">
                  {displayed}
                  {hasMore ? ' …' : ''}
                </span>
              </Link>
            )
          })}
        </div>
      )}
    </main>
  )
}
