export default function HomePage() {
  return (
    <main className="home-page">
      <div className="home-page__brand">
        <span className="home-page__brand-icon" aria-hidden="true">
          🍊
        </span>
        <h1 className="home-page__brand-name">Orange</h1>
      </div>

      <section className="home-page__welcome">
        <h2 className="home-page__welcome-title">欢迎使用 Orange</h2>
        <p className="home-page__welcome-text">
          一个记录你收藏的英文单词与相似词族的小工具。从左侧菜单开始探索。
        </p>
      </section>
    </main>
  )
}
