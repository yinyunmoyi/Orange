import { Link, NavLink, useLocation } from 'react-router-dom'

export default function Sidebar() {
  const { pathname } = useLocation()

  const isWordsActive =
    pathname.startsWith('/console/list') || pathname.startsWith('/console/word')
  const isSimilarActive =
    pathname.startsWith('/console/same_list') ||
    pathname.startsWith('/console/same_group')
  const isSentenceActive = pathname.startsWith('/console/sentence')
  const isVideoActive = pathname.startsWith('/console/videos')

  const linkClass = (active) =>
    `sidebar__link${active ? ' sidebar__link--active' : ''}`

  return (
    <aside className="app-layout__sidebar">
      <Link to="/" className="sidebar__brand">
        <span className="sidebar__brand-icon" aria-hidden="true">
          🍊
        </span>
        <span className="sidebar__brand-name">Orange</span>
      </Link>

      <nav className="sidebar__nav">
        <NavLink to="/console/list" className={() => linkClass(isWordsActive)}>
          单词
        </NavLink>
        <NavLink
          to="/console/same_list"
          className={() => linkClass(isSimilarActive)}
        >
          相似词
        </NavLink>
        <NavLink
          to="/console/sentence"
          className={() => linkClass(isSentenceActive)}
        >
          长难句
        </NavLink>
        <NavLink
          to="/console/videos"
          className={() => linkClass(isVideoActive)}
        >
          视频管理
        </NavLink>
      </nav>
    </aside>
  )
}
