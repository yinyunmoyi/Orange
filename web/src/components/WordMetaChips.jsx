import './WordMetaChips.css'

// 从后端已规范化的 pos 字段（形如 "adj.:35/v.:20"）中挑首个词性显示。
// 兼容原始未规范化的字母编码作为兜底。
function extractPrimaryPos(pos) {
  if (!pos) return ''
  const first = String(pos).split('/')[0] || ''
  const code = first.split(':')[0].trim()
  if (!code) return ''
  switch (code.toLowerCase()) {
    case 'n':
      return 'n.'
    case 'v':
      return 'v.'
    case 'j':
    case 'a':
    case 's':
      return 'adj.'
    case 'r':
      return 'adv.'
    case 'p':
      return 'pron.'
    case 'c':
      return 'conj.'
    case 'i':
      return 'prep.'
    case 'u':
      return 'interj.'
    case 'd':
      return 'det.'
    case 'm':
      return 'num.'
    case 't':
      return 'to'
    default:
      return code
  }
}

// zk/gk/ky 是中国考试词表缩写，前端展示时替换成中文全称。
// 其他 tag（cet4/cet6/toefl/ielts/gre/ospd 等）保持大写原样。
function displayTag(tag) {
  switch (String(tag).trim().toLowerCase()) {
    case 'zk':
      return '初中'
    case 'gk':
      return '高中'
    case 'ky':
      return '考研'
    default:
      return String(tag).toUpperCase()
  }
}

export default function WordMetaChips({ pos, oxford, tags, showPos = true }) {
  const primaryPos = showPos ? extractPrimaryPos(pos) : ''
  const tagList = Array.isArray(tags) ? tags.filter(Boolean) : []
  const hasAnything = primaryPos || Number(oxford) === 1 || tagList.length > 0
  if (!hasAnything) return null

  return (
    <div className="word-meta-chips">
      {primaryPos && (
        <span
          className="word-meta-chip word-meta-chip--pos"
          title="主要词性"
        >
          {primaryPos}
        </span>
      )}
      {Number(oxford) === 1 && (
        <span
          className="word-meta-chip word-meta-chip--oxford"
          title="牛津核心 3000"
        >
          牛津
        </span>
      )}
      {tagList.map((tag) => (
        <span
          key={tag}
          className="word-meta-chip word-meta-chip--tag"
          title={`标签：${tag}`}
        >
          {displayTag(tag)}
        </span>
      ))}
    </div>
  )
}
