import { Outlet, useLocation } from 'react-router-dom'
import Sidebar from './Sidebar.jsx'
import './AppLayout.css'

export default function AppLayout() {
  const { pathname } = useLocation()
  const videoLayout = pathname.startsWith('/console/videos')

  return (
    <div className={`app-layout${videoLayout ? ' app-layout--video' : ''}`}>
      <Sidebar />
      <main className="app-layout__content">
        <div
          className={`app-layout__content-inner${videoLayout ? ' app-layout__content-inner--video' : ''}`}
        >
          <Outlet />
        </div>
      </main>
    </div>
  )
}
