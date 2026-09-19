// src/components/HelloWorld.jsx
function HelloWorld({ name }) {
  return <h2>欢迎，{name || '陌生人'}！</h2>
}

export default HelloWorld