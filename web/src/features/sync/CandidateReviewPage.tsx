import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  Check,
  CircleHelp,
  CircleSlash2,
  FilePlus2,
  LoaderCircle,
  RefreshCw,
  SearchCheck,
  TriangleAlert,
} from 'lucide-react'
import { useId, useState } from 'react'
import { Link } from 'react-router-dom'

import {
  confirmSyncCandidate,
  createCustomSyncCandidate,
  getSyncCandidates,
  ignoreSyncCandidate,
  rematchSyncCandidate,
} from '../../api/client'
import type { MediaType, SyncCandidate, SyncCandidateStatus, SyncMatchOption } from '../../api/types'

const reviewStatuses = new Set<SyncCandidateStatus>(['exact', 'possible', 'unmatched', 'conflict'])

type CandidateAction =
  | { kind: 'confirm' }
  | { kind: 'rematch'; mediaId: string; episodeId: string }
  | { kind: 'ignore' }
  | { kind: 'custom'; title: string; mediaType: MediaType; year: string }

export function CandidateReviewPage() {
  const candidates = useQuery({
    queryKey: ['sync-candidates'],
    queryFn: ({ signal }) => getSyncCandidates(signal),
  })
  const pendingCandidates = candidates.data?.filter((candidate) => reviewStatuses.has(candidate.status)) ?? []

  return (
    <div className="page sync-review-page">
      <header className="sync-review-header">
        <div>
          <Link to="/settings"><ArrowLeft aria-hidden="true" size={16} />返回设置</Link>
          <h1>同步候选</h1>
          <p>只有明确且不与个人记录冲突的事件会自动落档。</p>
        </div>
        {candidates.data ? <span>{pendingCandidates.length} 条待核对</span> : null}
      </header>

      {candidates.isPending ? (
        <div className="sync-review-skeleton skeleton" aria-label="正在加载同步候选" />
      ) : candidates.isError ? (
        <div className="sync-review-error" role="alert">
          <span><TriangleAlert aria-hidden="true" size={18} />无法读取同步候选</span>
          <button type="button" onClick={() => void candidates.refetch()}>
            <RefreshCw aria-hidden="true" size={16} />重试
          </button>
        </div>
      ) : pendingCandidates.length === 0 ? (
        <div className="sync-review-empty">
          <Check aria-hidden="true" size={22} />
          <p>没有需要核对的同步记录</p>
          <span>新的冲突或无法匹配事件会显示在这里。</span>
        </div>
      ) : (
        <>
          <div className="sync-bulk-actions">
            <BulkCandidateActions candidates={pendingCandidates} onDone={() => void candidates.refetch()} />
          </div>
          <ol className="sync-candidate-list" aria-label="待核对同步候选">
            {pendingCandidates.map((candidate) => <CandidateItem key={candidate.id} candidate={candidate} />)}
          </ol>
        </>
      )}
    </div>
  )
}

function CandidateItem({ candidate }: { candidate: SyncCandidate }) {
  const queryClient = useQueryClient()
  const titleID = useId()
  const [selectedTarget, setSelectedTarget] = useState(targetKey(
    candidate.mediaId
      ? { mediaId: candidate.mediaId, ...(candidate.episodeId ? { episodeId: candidate.episodeId } : {}) }
      : undefined,
  ))
  const [customOpen, setCustomOpen] = useState(false)
  const [customTitle, setCustomTitle] = useState(candidate.event.title)
  const [customYear, setCustomYear] = useState(candidate.event.year?.toString() ?? '')
  const [message, setMessage] = useState('')
  const mutation = useMutation({
    mutationFn: (action: CandidateAction) => runCandidateAction(candidate.id, action),
    onMutate: () => setMessage(''),
    onSuccess: () => {
      setMessage('同步记录已处理')
      void queryClient.invalidateQueries({ queryKey: ['sync-candidates'] })
      void queryClient.invalidateQueries({ queryKey: ['sync-status'] })
    },
  })
  const selectedOption = candidate.options.find((option) => targetKey(option) === selectedTarget)
  const customMediaType: MediaType = candidate.event.mediaType === 'episode' ? 'tv' : 'movie'

  return (
    <li>
      <article className="sync-candidate" aria-labelledby={titleID}>
        <div className="sync-candidate-summary">
          <CandidateStatusLabel status={candidate.status} />
          <div>
            <h2 id={titleID}>{candidate.event.title}</h2>
            <p>{candidateEventMeta(candidate)}</p>
          </div>
          <time dateTime={candidate.event.playedAt}>{formatCandidateDate(candidate.event.playedAt)}</time>
        </div>

        <ul className="sync-evidence" aria-label="匹配依据">
          {candidate.evidence.map((evidence) => <li key={evidence.code}>{evidence.text}</li>)}
        </ul>

        {candidate.options.length > 0 ? (
          <fieldset className="sync-match-options">
            <legend>候选条目</legend>
            {candidate.options.map((option, index) => (
              <label key={targetKey(option)} className="sync-match-option">
                <input
                  type="radio"
                  name={`candidate-${candidate.id}`}
                  value={targetKey(option)}
                  checked={selectedTarget === targetKey(option)}
                  onChange={() => setSelectedTarget(targetKey(option))}
                />
                <span className="sync-option-preview">
                  <span className="sync-option-index">候选 {index + 1}</span>
                  <strong>{option.title}</strong>
                  <span>{optionMeta(option)}</span>
                  <span className="sync-option-local-status">
                    {option.mediaType === 'tv' ? '剧集' : '电影'}
                    {option.year ? ` · ${option.year}` : ''}
                    {option.mediaId ? ' · 已有本地条目' : ''}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
        ) : (
          <p className="sync-option-empty">暂无本地候选；可忽略或创建自定义条目。</p>
        )}

        <div className="sync-candidate-actions">
          {candidate.mediaId ? (
            <button
              className="primary"
              type="button"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate({ kind: 'confirm' })}
            >
              <Check aria-hidden="true" size={16} />确认此匹配
            </button>
          ) : null}
          {candidate.options.length > 0 && !candidate.mediaId ? (
            <button
              className="primary"
              type="button"
              disabled={!selectedOption || mutation.isPending}
              onClick={() => selectedOption && mutation.mutate({
                kind: 'rematch', mediaId: selectedOption.mediaId, episodeId: selectedOption.episodeId ?? '',
              })}
            >
              <SearchCheck aria-hidden="true" size={16} />使用所选匹配
            </button>
          ) : null}
          <button type="button" disabled={mutation.isPending} onClick={() => mutation.mutate({ kind: 'ignore' })}>
            <CircleSlash2 aria-hidden="true" size={16} />忽略此事件
          </button>
          <button
            type="button"
            aria-expanded={customOpen}
            disabled={mutation.isPending}
            onClick={() => setCustomOpen((open) => !open)}
          >
            <FilePlus2 aria-hidden="true" size={16} />创建自定义条目
          </button>
        </div>

        {customOpen ? (
          <form className="sync-custom-form" onSubmit={(event) => {
            event.preventDefault()
            mutation.mutate({
              kind: 'custom', title: customTitle.trim(), mediaType: customMediaType, year: customYear.trim(),
            })
          }}>
            <label>
              <span>自定义标题</span>
              <input
                autoFocus
                required
                value={customTitle}
                onChange={(event) => setCustomTitle(event.target.value)}
              />
            </label>
            <label>
              <span>年份（可选）</span>
              <input
                inputMode="numeric"
                pattern="[0-9]{4}"
                value={customYear}
                onChange={(event) => setCustomYear(event.target.value)}
              />
            </label>
            <span className="sync-custom-type">{customMediaType === 'tv' ? '剧集' : '电影'}</span>
            <button type="submit" disabled={mutation.isPending || customTitle.trim() === ''}>
              {mutation.isPending
                ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                : <Check aria-hidden="true" size={16} />}
              {mutation.isPending ? '正在保存' : '保存并导入'}
            </button>
          </form>
        ) : null}

        {mutation.isError ? <p className="sync-candidate-error" role="alert">保存失败，当前选择和输入已保留。</p> : null}
        {message ? <p className="sync-candidate-message" role="status">{message}</p> : null}
      </article>
    </li>
  )
}

function CandidateStatusLabel({ status }: { status: SyncCandidateStatus }) {
  if (status === 'conflict') {
    return <span className="sync-candidate-state conflict"><TriangleAlert aria-hidden="true" size={16} />冲突</span>
  }
  if (status === 'unmatched') {
    return <span className="sync-candidate-state unmatched"><CircleSlash2 aria-hidden="true" size={16} />无法匹配</span>
  }
  if (status === 'exact') {
    return <span className="sync-candidate-state exact"><Check aria-hidden="true" size={16} />精确匹配</span>
  }
  return <span className="sync-candidate-state possible"><CircleHelp aria-hidden="true" size={16} />可能匹配</span>
}

function runCandidateAction(candidateID: string, action: CandidateAction) {
  if (action.kind === 'confirm') return confirmSyncCandidate(candidateID)
  if (action.kind === 'rematch') return rematchSyncCandidate(candidateID, action.mediaId, action.episodeId)
  if (action.kind === 'ignore') return ignoreSyncCandidate(candidateID)
  return createCustomSyncCandidate(candidateID, {
    title: action.title, mediaType: action.mediaType, year: action.year,
  })
}

function targetKey(option?: Pick<SyncMatchOption, 'mediaId' | 'episodeId'>) {
  return option ? `${option.mediaId}:${option.episodeId ?? ''}` : ''
}

function optionMeta(option: SyncMatchOption) {
  const values = [
    option.originalTitle && option.originalTitle !== option.title ? `原名 ${option.originalTitle}` : '',
    option.year,
    option.mediaType === 'tv' ? '剧集' : '电影',
  ].filter(Boolean)
  return values.length > 0 ? `（${values.join('，')}）` : ''
}

function candidateEventMeta(candidate: SyncCandidate) {
  const values = [candidate.event.mediaType === 'episode' ? '剧集' : '电影']
  if (candidate.event.year) values.push(candidate.event.year.toString())
  if (candidate.event.mediaType === 'episode') {
    values.push(`S${pad(candidate.event.seasonNumber)}E${pad(candidate.event.episodeNumber)}`)
  }
  return values.join(' · ')
}

function pad(value?: number) {
  return String(value ?? 0).padStart(2, '0')
}

function formatCandidateDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}


function BulkCandidateActions({ candidates, onDone }: { candidates: SyncCandidate[]; onDone: () => void }) {
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')
  const confirmable = candidates.filter((candidate) => candidate.status === 'exact' && candidate.mediaId)
  const ignorable = candidates.filter((candidate) => candidate.status === 'exact' || candidate.status === 'possible')
  if (confirmable.length === 0 && ignorable.length === 0) return null

  const run = async (kind: 'confirm' | 'ignore') => {
    const targets = kind === 'confirm' ? confirmable : ignorable
    if (targets.length === 0) return
    setPending(true)
    setMessage('')
    try {
      for (const candidate of targets) {
        if (kind === 'confirm') await confirmSyncCandidate(candidate.id)
        else await ignoreSyncCandidate(candidate.id)
      }
      setMessage(kind === 'confirm' ? `已确认 ${targets.length} 条匹配` : `已忽略 ${targets.length} 条候选`)
      onDone()
    } catch {
      setMessage(kind === 'confirm' ? '批量确认失败，请稍后重试' : '批量忽略失败，请稍后重试')
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="sync-bulk-action-group">
      {confirmable.length > 0 ? (
        <button type="button" disabled={pending} onClick={() => void run('confirm')}>
          {pending ? '处理中' : `批量确认 ${confirmable.length} 条精确匹配`}
        </button>
      ) : null}
      {ignorable.length > 0 ? (
        <button type="button" disabled={pending} onClick={() => void run('ignore')}>
          {pending ? '处理中' : `批量忽略 ${ignorable.length} 条精确/可能匹配`}
        </button>
      ) : null}
      {message ? <p role="status">{message}</p> : null}
    </div>
  )
}
