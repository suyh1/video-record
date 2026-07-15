import { ArrowLeft, Search } from 'lucide-react'
import { Link } from 'react-router-dom'

import { BrandMark } from './BrandMark'

export function NotFoundPage({ onSearch }: { onSearch: () => void }) {
  return (
    <section className="not-found-page" aria-labelledby="not-found-heading">
      <span className="not-found-mark" aria-hidden="true">
        <BrandMark size={36} />
      </span>
      <h1 id="not-found-heading">没有找到这份档案</h1>
      <p>这个地址没有对应的影视、记录或设置页面。</p>
      <div className="not-found-actions">
        <Link className="button-secondary" to="/">
          <ArrowLeft aria-hidden="true" size={18} />
          返回首页
        </Link>
        <button className="button-primary" type="button" onClick={onSearch}>
          <Search aria-hidden="true" size={18} />
          搜索影视
        </button>
      </div>
    </section>
  )
}
