import type { TMDBCastMember } from '../../api/types'

type CastStripProps = {
  cast?: TMDBCastMember[]
  pending: boolean
  error: boolean
  linked: boolean
  onRetry: () => void
}

export function CastStrip({ cast = [], pending, error, linked, onRetry }: CastStripProps) {
  return (
    <section className="cast-section" aria-labelledby="cast-heading">
      <div className="cast-heading">
        <div><h2 id="cast-heading">主要演员</h2><p>演员资料来自 TMDB</p></div>
      </div>
      {pending ? (
        <div className="cast-strip cast-strip-skeleton" aria-label="正在加载主要演员">
          {Array.from({ length: 6 }, (_, index) => <div className="skeleton" key={index} />)}
        </div>
      ) : null}
      {error ? (
        <div className="cast-message"><p>演员资料暂时不可用</p><button type="button" onClick={onRetry}>重新获取演员</button></div>
      ) : null}
      {!pending && !error && linked && cast.length === 0 ? <p className="quiet-empty">TMDB 暂无演员资料</p> : null}
      {!pending && !error && !linked ? <p className="quiet-empty">关联 TMDB 后可显示演员</p> : null}
      {!pending && !error && cast.length > 0 ? (
        <ul className="cast-strip">
          {cast.map((member) => (
            <li key={`${member.id}-${member.character}`}>
              <div className="cast-portrait">
                {member.profilePath ? (
                  <img src={profileURL(member.profilePath)} alt={`${member.name}${member.character ? ` 饰 ${member.character}` : ''}`} loading="lazy" />
                ) : <span aria-hidden="true">{initial(member.name)}</span>}
              </div>
              <strong>{member.name}</strong>
              <span>{member.character || '角色未知'}</span>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}

function profileURL(path: string) {
  if (/^https?:\/\//.test(path)) return path
  return `https://image.tmdb.org/t/p/w300${path}`
}

function initial(name: string) {
  return Array.from(name.trim())[0] ?? '演'
}
