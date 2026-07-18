import { useMutation, useQuery } from '@tanstack/react-query'
import { Bookmark, Check, ChevronDown, ChevronUp, CircleStop, LoaderCircle, Play } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { APIError, getViewingMethods, updateCurrentRound, type UpdateCurrentRoundPayload } from '../../api/client'
import type { CurrentRound, HouseholdMember, RecordStatus } from '../../api/types'
import { fromDateTimeLocalValue, isFutureDateTimeLocalValue, toDateTimeLocalValue } from '../../lib/dateTime'
import { RatingPicker } from './RatingPicker'

type RoundRecordFormProps = {
  round: CurrentRound
  now: Date
  participants?: HouseholdMember[]
  onSaved: (round: CurrentRound) => void
}

export function RoundRecordForm({ round, now, participants = [], onSaved }: RoundRecordFormProps) {
  const [expanded, setExpanded] = useState(hasOptionalRecord(round))
  const [message, setMessage] = useState<{ kind: 'error' | 'conflict'; text: string } | null>(null)
  const [conflictVersion, setConflictVersion] = useState<number | null>(null)
  const [statusMessage, setStatusMessage] = useState('')
  const errorID = useId()
  const form = useForm<FormValues>({ defaultValues: formValuesFromRound(round) })
  const status = form.watch('status')
  const viewingMethod = form.watch('viewingMethod')
  const viewingMethods = useQuery({
    queryKey: ['viewing-methods'],
    queryFn: ({ signal }) => getViewingMethods(signal),
    enabled: expanded,
  })
  const roundIdentity = `${round.mediaId}:${round.seasonNumber ?? 'movie'}:${round.roundNumber}`
  const previousRoundIdentity = useRef(roundIdentity)
  const mutation = useMutation({
    mutationFn: ({ payload, version }: MutationVariables) => updateCurrentRound(
      round.mediaId,
      round.seasonNumber ?? undefined,
      version,
      payload,
    ),
    onSuccess: (nextRound) => {
      setMessage(null)
      setConflictVersion(null)
      setStatusMessage(statusSaveMessage(nextRound.status))
      form.reset(formValuesFromRound(nextRound))
      setExpanded(hasOptionalRecord(nextRound))
      onSaved(nextRound)
    },
    onError: (error) => {
      setStatusMessage('')
      if (error instanceof APIError && error.status === 409 && error.code === 'version_conflict') {
        setConflictVersion(parseETag(error.etag))
        setMessage({ kind: 'conflict', text: '记录已在其他位置更新。你的输入仍保留在此处。' })
        return
      }
      setMessage({ kind: 'error', text: '保存失败，请检查连接后重试。你的输入已保留。' })
    },
  })

  useEffect(() => {
    if (previousRoundIdentity.current === roundIdentity) return
    previousRoundIdentity.current = roundIdentity
    form.reset(formValuesFromRound(round))
    setExpanded(hasOptionalRecord(round))
    setMessage(null)
    setConflictVersion(null)
    setStatusMessage('')
  }, [form, round, roundIdentity])

  const submit = form.handleSubmit((values) => submitValues(values, round.version))
  const retryConflict = form.handleSubmit((values) => {
    if (conflictVersion !== null) submitValues(values, conflictVersion)
  })

  function submitValues(values: FormValues, version: number) {
    form.clearErrors()
    const parsed = formSchema(round.seasonNumber === null, now).safeParse(values)
    if (!parsed.success) {
      const first = parsed.error.issues[0]
      if (first?.path[0]) {
        form.setError(first.path[0] as keyof FormValues, { message: first.message }, { shouldFocus: true })
      }
      return
    }
    setMessage(null)
    setStatusMessage('')
    mutation.mutate({
      payload: toPayload(parsed.data, expanded, round.seasonNumber === null, round.status),
      version,
    })
  }

  function selectStatus(nextStatus: RecordStatus) {
    form.setValue('status', nextStatus, { shouldDirty: true })
    if (nextStatus === 'completed' && !form.getValues('watchedAt')) {
      form.setValue('watchedAt', toDateTimeLocalValue(now), { shouldDirty: true })
    }
    if (nextStatus === 'watching' && !form.getValues('startedAt')) {
      form.setValue('startedAt', toDateTimeLocalValue(now), { shouldDirty: true })
    }
  }

  const watchedAtError = form.formState.errors.watchedAt
  const startedAtError = form.formState.errors.startedAt

  return (
    <section className="round-record" aria-labelledby={`${errorID}-heading`}>
      <div className="details-section-heading round-record-heading">
        <div>
          <h2 id={`${errorID}-heading`}>
            {round.seasonNumber === null ? '个人记录' : `第 ${round.seasonNumber} 季个人记录`}
          </h2>
          <p>评分和私人笔记仅自己可见</p>
        </div>
      </div>

      <form className="round-record-form" onSubmit={submit} noValidate>
        {round.seasonNumber === null ? (
          <fieldset>
            <legend>观看状态</legend>
            <div className="status-control" role="radiogroup" aria-label="观看状态">
              {statusOptions.map(({ value, label, icon: Icon }) => (
                <button
                  key={value}
                  className={status === value ? 'selected' : ''}
                  type="button"
                  role="radio"
                  aria-checked={status === value}
                  onClick={() => selectStatus(value)}
                >
                  <Icon aria-hidden="true" size={16} />
                  {label}
                </button>
              ))}
            </div>
          </fieldset>
        ) : (
          <div className="round-status-summary">
            <span>本季状态</span>
            <strong>{statusLabels[round.status]}</strong>
          </div>
        )}

        {round.seasonNumber === null && status === 'watching' ? (
          <label className="form-field watched-at-field">
            <span>开始观看时间</span>
            <input
              type="datetime-local"
              step="1"
              max={toDateTimeLocalValue(now)}
              aria-invalid={Boolean(startedAtError)}
              aria-describedby={startedAtError ? `${errorID}-started-at-error` : undefined}
              {...form.register('startedAt')}
            />
            {startedAtError ? <small id={`${errorID}-started-at-error`}>{startedAtError.message}</small> : null}
          </label>
        ) : null}

        {round.seasonNumber === null && status === 'completed' ? (
          <label className="form-field watched-at-field">
            <span>完成观看时间</span>
            <input
              type="datetime-local"
              step="1"
              max={toDateTimeLocalValue(now)}
              aria-invalid={Boolean(watchedAtError)}
              aria-describedby={watchedAtError ? `${errorID}-watched-at-error` : undefined}
              {...form.register('watchedAt')}
            />
            {watchedAtError ? <small id={`${errorID}-watched-at-error`}>{watchedAtError.message}</small> : null}
          </label>
        ) : null}

        <button
          className="expand-record-fields"
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? <ChevronUp aria-hidden="true" size={16} /> : <ChevronDown aria-hidden="true" size={16} />}
          {expanded ? '收起记录选项' : '更多记录选项'}
        </button>

        {expanded ? (
          <div className="optional-record-fields">
            <div className="form-field rating-picker-field">
              <RatingPicker
                value={form.watch('rating')}
                onChange={(next) => form.setValue('rating', next, { shouldDirty: true, shouldValidate: true })}
                {...(form.formState.errors.rating?.message
                  ? { error: form.formState.errors.rating.message }
                  : {})}
              />
            </div>
            <div className="form-field viewing-method-field">
              <label className="compact-field">
                <span>观看方式</span>
                <input type="text" maxLength={80} placeholder="如：影院、家庭电视" {...form.register('viewingMethod')} />
              </label>
              {viewingMethods.data && viewingMethods.data.length > 0 ? (
                <div className="viewing-method-chips" role="group" aria-label="常用观看方式">
                  {viewingMethods.data.map((method) => (
                    <button
                      key={method}
                      type="button"
                      className={viewingMethod.trim() === method ? 'selected' : ''}
                      aria-pressed={viewingMethod.trim() === method}
                      onClick={() => form.setValue('viewingMethod', method, { shouldDirty: true })}
                    >
                      {method}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
            {round.seasonNumber === null && status === 'completed' && participants.length > 0 ? (
              <fieldset className="participant-fieldset">
                <legend>共同观看者</legend>
                <div className="participant-options">
                  {participants.map((participant) => (
                    <label key={participant.id}>
                      <input type="checkbox" value={participant.id} {...form.register('participantIds')} />
                      <span>{participant.username}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            ) : null}
            <label className="form-field note-field">
              <span>私人笔记</span>
              <textarea rows={4} maxLength={5000} {...form.register('note')} />
            </label>
          </div>
        ) : null}

        {message ? (
          <div className={`form-message ${message.kind}`} role="alert">
            <p>{message.text}</p>
            {message.kind === 'conflict' && conflictVersion !== null ? (
              <button type="button" onClick={retryConflict} disabled={mutation.isPending}>
                使用最新版本重试
              </button>
            ) : null}
          </div>
        ) : null}

        {statusMessage ? <div className="save-toast" role="status"><span>{statusMessage}</span></div> : null}

        <div className="form-actions">
          <button
            className="primary-button"
            type="submit"
            disabled={mutation.isPending || (round.seasonNumber === null && status === 'none')}
          >
            {mutation.isPending ? (
              <>
                <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                正在保存
              </>
            ) : (
              <>
                <Check aria-hidden="true" size={16} />
                保存记录
              </>
            )}
          </button>
        </div>
      </form>
    </section>
  )
}

type FormValues = {
  status: RecordStatus
  watchedAt: string
  startedAt: string
  rating: string
  note: string
  viewingMethod: string
  participantIds: string[]
}

type MutationVariables = {
  payload: UpdateCurrentRoundPayload
  version: number
}

const statusOptions = [
  { value: 'wishlist', label: '想看', icon: Bookmark },
  { value: 'watching', label: '在看', icon: Play },
  { value: 'completed', label: '看过', icon: Check },
  { value: 'dropped', label: '弃看', icon: CircleStop },
] satisfies Array<{ value: RecordStatus; label: string; icon: typeof Bookmark }>

const statusLabels: Record<RecordStatus, string> = {
  none: '未看',
  wishlist: '想看',
  watching: '在看',
  completed: '已看完',
  dropped: '已弃看',
}

function statusSaveMessage(status: RecordStatus) {
  switch (status) {
    case 'wishlist':
      return '已标为想看'
    case 'watching':
      return '已标为在看'
    case 'completed':
      return '已标为看过'
    case 'dropped':
      return '已标为弃看'
    default:
      return '记录已保存'
  }
}

function formSchema(movie: boolean, now: Date) {
  return z
    .object({
      status: z.enum(['none', 'wishlist', 'watching', 'completed', 'dropped']),
      watchedAt: z.string(),
      startedAt: z.string(),
      rating: z.string().refine(
        (value) => {
          if (value === '') return true
          const number = Number(value)
          if (Number.isNaN(number) || number < 0 || number > 10) return false
          return Math.abs(number * 2 - Math.round(number * 2)) < 1e-9
        },
        '评分必须在 0 到 10 之间，步进 0.5',
      ),
      note: z.string().max(5000, '笔记不能超过 5000 字'),
      viewingMethod: z.string().max(80, '观看方式不能超过 80 字'),
      participantIds: z.array(z.string().min(1)),
    })
    .superRefine((value, context) => {
      if (!movie) return
      if (value.status === 'watching' && value.startedAt) {
        if (fromDateTimeLocalValue(value.startedAt) === null) {
          context.addIssue({ code: 'custom', path: ['startedAt'], message: '请输入有效的开始观看时间' })
        } else if (isFutureDateTimeLocalValue(value.startedAt, now)) {
          context.addIssue({ code: 'custom', path: ['startedAt'], message: '开始观看时间不能晚于当前时间' })
        }
      }
      if (value.status !== 'completed') return
      if (!value.watchedAt) {
        context.addIssue({ code: 'custom', path: ['watchedAt'], message: '请选择完成观看时间' })
        return
      }
      if (fromDateTimeLocalValue(value.watchedAt) === null) {
        context.addIssue({ code: 'custom', path: ['watchedAt'], message: '请输入有效的完成观看时间' })
        return
      }
      if (isFutureDateTimeLocalValue(value.watchedAt, now)) {
        context.addIssue({ code: 'custom', path: ['watchedAt'], message: '完成观看时间不能晚于当前时间' })
      }
    })
}

function toPayload(
  values: FormValues,
  expanded: boolean,
  movie: boolean,
  projectedStatus: RecordStatus,
): UpdateCurrentRoundPayload {
  const payload: UpdateCurrentRoundPayload = { status: movie ? values.status : projectedStatus }
  if (movie && values.status === 'watching' && values.startedAt) {
    const startedAt = fromDateTimeLocalValue(values.startedAt)
    if (startedAt) payload.startedAt = startedAt.toISOString()
  }
  if (movie && values.status === 'completed') {
    const watchedAt = fromDateTimeLocalValue(values.watchedAt)
    if (watchedAt) payload.watchedAt = watchedAt.toISOString()
    payload.participantIds = values.participantIds
  }
  if (expanded) {
    payload.rating = values.rating === '' ? null : Number(values.rating)
    payload.note = values.note.trim() || null
    payload.viewingMethod = values.viewingMethod.trim() || null
  }
  return payload
}

function formValuesFromRound(round: CurrentRound): FormValues {
  return {
    status: round.status,
    watchedAt: round.watchedAt ? toDateTimeLocalValue(new Date(round.watchedAt)) : '',
    startedAt: round.startedAt ? toDateTimeLocalValue(new Date(round.startedAt)) : '',
    rating: round.rating === null ? '' : String(round.rating),
    note: round.note ?? '',
    viewingMethod: round.viewingMethod ?? '',
    participantIds: [...round.participantIds],
  }
}

function hasOptionalRecord(round: CurrentRound) {
  return round.rating !== null || Boolean(round.note || round.viewingMethod)
}

function parseETag(etag: string | null) {
  if (!etag) return null
  const version = Number(etag.replaceAll('"', ''))
  return Number.isInteger(version) && version >= 0 ? version : null
}
