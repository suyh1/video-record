import { useMutation } from '@tanstack/react-query'
import { CircleAlert, CircleCheck, ExternalLink, LoaderCircle, RefreshCw } from 'lucide-react'

import { APIError, testTMDBConnectivity } from '../../api/client'

type TmdbStatusProps = {
  configured: boolean
}

export function TmdbStatus({ configured }: TmdbStatusProps) {
  const StatusIcon = configured ? CircleCheck : CircleAlert
  const connectivity = useMutation({ mutationFn: testTMDBConnectivity })

  return (
    <section className="integration-status" aria-labelledby="tmdb-heading">
      <div className="integration-heading">
        <div>
          <p className="page-kicker">元数据来源</p>
          <h2 id="tmdb-heading">TMDB</h2>
        </div>
        <div className="integration-status-actions">
          <div className={`status-label ${configured ? 'configured' : 'unconfigured'}`} role="status">
            <StatusIcon aria-hidden="true" size={18} strokeWidth={1.8} />
            <span>{configured ? 'TMDB 已配置' : 'TMDB 未配置'}</span>
          </div>
          {configured ? (
            <button
              className="tmdb-connectivity-button"
              type="button"
              disabled={connectivity.isPending}
              onClick={() => connectivity.mutate()}
            >
              {connectivity.isPending
                ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                : <RefreshCw aria-hidden="true" size={16} />}
              {connectivity.isPending ? '正在测试' : '测试连通'}
            </button>
          ) : null}
        </div>
      </div>

      {connectivity.isSuccess ? (
        <p className="integration-connectivity-result success" role="status">TMDB 连通正常</p>
      ) : null}
      {connectivity.isError ? (
        <p className="integration-connectivity-result error" role="alert">{connectivityErrorMessage(connectivity.error)}</p>
      ) : null}
      {!configured && <p className="integration-guidance">需要由服务端设置环境变量 TMDB_READ_ACCESS_TOKEN</p>}

      <TmdbAttribution />
    </section>
  )
}

function connectivityErrorMessage(error: Error) {
  if (!(error instanceof APIError)) return '测试 TMDB 连通失败，请稍后重试。'
  switch (error.code) {
    case 'tmdb_unauthorized':
      return 'TMDB 令牌无效，请检查服务端配置。'
    case 'tmdb_timeout':
      return '连接 TMDB 超时，请检查服务端代理或网络设置。'
    case 'tmdb_rate_limited':
      return 'TMDB 请求受限，请稍后重试。'
    case 'tmdb_unavailable':
      return '无法连接 TMDB，请检查服务端代理或网络设置。'
    case 'tmdb_not_configured':
      return 'TMDB 尚未配置，请检查服务端环境变量。'
    default:
      return '测试 TMDB 连通失败，请稍后重试。'
  }
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
