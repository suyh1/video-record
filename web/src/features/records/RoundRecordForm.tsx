import { useMutation } from '@tanstack/react-query'
import { Bookmark, Check, ChevronDown, ChevronUp, CircleStop, LoaderCircle, Play } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { APIError, updateCurrentRound, type UpdateCurrentRoundPayload } from '../../api/client'
import type { CurrentRound, HouseholdMember, RecordStatus } from '../../api/types'
import { fromDateTimeLocalValue, isFutureDateTimeLocalValue, toDateTimeLocalValue } from '../../lib/dateTime'

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
  const [saved, setSaved] = useState(false)
  const errorID = useId()
  const form = useForm<FormValues>({ defaultValues: formValuesFromRound(round) })
  const status = form.watch('status')
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
      setSaved(true)
      form.reset(formValuesFromRound(nextRound))
      setExpanded(hasOptionalRecord(nextRound))
      onSaved(nextRound)
    },
    onError: (error) => {
      setSaved(false)
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
    setSaved(false)
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
    setSaved(false)
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
  }

  const watchedAtError = form.formState.errors.watchedAt

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
            <label className="form-field compact-field">
              <span>评分</span>
              <span className="rating-input">
                <input
                  aria-label="评分"
                  aria-invalid={Boolean(form.formState.errors.rating)}
                  inputMode="decimal"
                  type="number"
                  min="0"
                  max="10"
                  step="0.1"
                  {...form.register('rating')}
                />
                <span aria-hidden="true">/ 10</span>
              </span>
              {form.formState.errors.rating ? <small>{form.formState.errors.rating.message}</small> : null}
            </label>
            <label className="form-field compact-field">
              <span>观看方式</span>
              <input type="text" maxLength={80} placeholder="如：影院、家庭电视" {...form.register('viewingMethod')} />
            </label>
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

        {saved ? <div className="save-toast" role="status"><span>记录已保存</span></div> : null}

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

function formSchema(movie: boolean, now: Date) {
  return z
    .object({
      status: z.enum(['none', 'wishlist', 'watching', 'completed', 'dropped']),
      watchedAt: z.string(),
      rating: z.string().refine(
        (value) => value === '' || (!Number.isNaN(Number(value)) && Number(value) >= 0 && Number(value) <= 10),
        '评分必须在 0 到 10 之间',
      ),
      note: z.string().max(5000, '笔记不能超过 5000 字'),
      viewingMethod: z.string().max(80, '观看方式不能超过 80 字'),
      participantIds: z.array(z.string().min(1)),
    })
    .superRefine((value, context) => {
      if (!movie || value.status !== 'completed') return
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
