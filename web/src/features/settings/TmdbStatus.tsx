import { CircleAlert, CircleCheck, ExternalLink } from 'lucide-react'

type TmdbStatusProps = {
  configured: boolean
}

export function TmdbStatus({ configured }: TmdbStatusProps) {
  const StatusIcon = configured ? CircleCheck : CircleAlert

  return (
    <section className="integration-status" aria-labelledby="tmdb-heading">
      <div className="integration-heading">
        <div>
          <p className="page-kicker">元数据来源</p>
          <h2 id="tmdb-heading">TMDB</h2>
        </div>
        <div className={`status-label ${configured ? 'configured' : 'unconfigured'}`} role="status">
          <StatusIcon aria-hidden="true" size={18} strokeWidth={1.8} />
          <span>{configured ? 'TMDB 已配置' : 'TMDB 未配置'}</span>
        </div>
      </div>

      {!configured && <p className="integration-guidance">需要由服务端设置环境变量 TMDB_READ_ACCESS_TOKEN</p>}

      <TmdbAttribution />
    </section>
  )
}

export function TmdbAttribution() {
  return (
    <p className="attribution">
      This product uses the TMDB API but is not endorsed or certified by TMDB.
      <a href="https://www.themoviedb.org/" target="_blank" rel="noreferrer">
        <span>访问 TMDB</span>
        <ExternalLink aria-hidden="true" size={15} strokeWidth={1.8} />
      </a>
    </p>
  )
}
