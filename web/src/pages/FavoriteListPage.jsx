import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import '../WordCard.css'
import { listFavorites } from '../api/word.js'

function formatCreatedAt(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export default function FavoriteListPage() {
  const [items, setItems] = useState([])
  const [totalWords, setTotalWords] = useState(null)
  const [loading, setLoading] = useState(true)
  const [errorMsg, setErrorMsg] = useState('')
  const [inputValue, setInputValue] = useState('')
  const [committedQuery, setCommittedQuery] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setErrorMsg('')
    listFavorites(committedQuery)
      .then((data) => {
        if (cancelled) return
        const nextItems = data.items || []
        setItems(nextItems)
        if (!committedQuery) {
          setTotalWords(
            nextItems.filter((item) => item.itemType === 'word').length,
          )
        }
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
  }, [committedQuery])

  const handleSubmit = (e) => {
    e.preventDefault()
    setCommittedQuery(inputValue.trim().toLowerCase())
  }

  return (
    <main className="favorite-list-page">
      <header className="favorite-list-page__header">
        <h1 className="favorite-list-page__title">
          我的收藏
          {totalWords !== null && (
            <span className="page-title__count">共 {totalWords} 个</span>
          )}
        </h1>
        <form className="favorite-list-page__toolbar" onSubmit={handleSubmit}>
          <input
            className="favorite-search-input"
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            placeholder="搜索已收藏的单词"
            aria-label="搜索已收藏的单词"
          />
          <button type="submit" className="favorite-search-btn">
            确定
          </button>
        </form>
      </header>

      {loading && <p className="favorite-empty">加载中…</p>}
      {!loading && errorMsg && (
        <p className="favorite-empty favorite-empty--error">加载失败：{errorMsg}</p>
      )}
      {!loading && !errorMsg && items.length === 0 && (
        <p className="favorite-empty">
          {committedQuery ? `没有匹配 "${committedQuery}" 的收藏词` : '还没有收藏任何单词'}
        </p>
      )}
      {!loading && items.length > 0 && (
        <div className="favorite-grid">
          {items.map((item) => (
            <Link
              key={`${item.itemType}:${item.itemId}`}
              to={`/console/word/${encodeURIComponent(item.word)}`}
              className="favorite-card"
            >
              <span className="favorite-card__word">{item.word}</span>
              {item.topMeaningText && (
                <span className="favorite-card__meaning">
                  {item.topMeaningPos && (
                    <span className="favorite-card__pos">{item.topMeaningPos}</span>
                  )}
                  {item.topMeaningText}
                </span>
              )}
              <span className="favorite-card__time">{formatCreatedAt(item.createdAt)}</span>
            </Link>
          ))}
        </div>
      )}
    </main>
  )
}
