import { useMutation } from '@tanstack/react-query'
import { Bookmark, Check, ChevronDown, ChevronUp, CircleStop, LoaderCircle, Play } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { APIError, updateRecord, type UpdateRecordPayload } from '../../api/client'
import type { HouseholdMember, RecordState, RecordStatus } from '../../api/types'

type QuickRecordFormProps = {
  record: RecordState
  now: Date
  participants?: HouseholdMember[]
  onSaved: (record: RecordState) => void
}

export function QuickRecordForm({ record, now, participants = [], onSaved }: QuickRecordFormProps) {
  const [expanded, setExpanded] = useState(Boolean(record.rating !== null || record.note || record.viewingMethod))
  const [message, setMessage] = useState<{ kind: 'error' | 'conflict'; text: string } | null>(null)
  const [conflictVersion, setConflictVersion] = useState<number | null>(null)
  const [savedChange, setSavedChange] = useState<{
    saved: RecordState
    previous: UpdateRecordPayload
    undoable: boolean
  } | null>(null)
  const form = useForm<FormValues>({
    defaultValues: {
      status: record.status,
      watchedDate: record.watchedAt?.slice(0, 10) ?? '',
      rating: record.rating === null ? '' : String(record.rating),
      note: record.note ?? '',
      viewingMethod: record.viewingMethod ?? '',
      participantIds: [],
    },
  })
  const status = form.watch('status')
  const mutation = useMutation({
    mutationFn: ({ payload, version }: MutationVariables) => updateRecord(record.mediaId, version, payload),
    onSuccess: (saved, variables) => {
      setMessage(null)
      setConflictVersion(null)
      if (variables.action === 'undo') {
        setSavedChange(null)
        form.reset(formValuesFromRecord(record))
      } else {
        setSavedChange({
          saved,
          previous: undoPayload(record, variables.payload),
          undoable: saved.status !== 'completed' && record.status !== 'completed',
        })
      }
      onSaved(saved)
    },
    onError: (error) => {
      if (error instanceof APIError && error.status === 409 && error.code === 'version_conflict') {
        setConflictVersion(parseETag(error.etag))
        setMessage({ kind: 'conflict', text: '记录已在其他位置更新。你的输入仍保留在此处。' })
        return
      }
      setMessage({ kind: 'error', text: '保存失败，请检查连接后重试。你的输入已保留。' })
    },
  })

  useEffect(() => {
    if (!savedChange) return
    const timeout = window.setTimeout(() => setSavedChange(null), 10_000)
    return () => window.clearTimeout(timeout)
  }, [savedChange])

  const submit = form.handleSubmit((values) => {
    const parsed = formSchema.safeParse(values)
    if (!parsed.success) {
      const first = parsed.error.issues[0]
      if (first?.path[0]) form.setError(first.path[0] as keyof FormValues, { message: first.message }, { shouldFocus: true })
      return
    }
    setMessage(null)
    mutation.mutate({ payload: toPayload(parsed.data, expanded), version: record.version, action: 'save' })
  })

  const retryConflict = form.handleSubmit((values) => {
    const parsed = formSchema.safeParse(values)
    if (parsed.success && conflictVersion !== null) {
      mutation.mutate({ payload: toPayload(parsed.data, expanded), version: conflictVersion, action: 'save' })
    }
  })

  const selectStatus = (nextStatus: RecordStatus) => {
    form.setValue('status', nextStatus, { shouldDirty: true })
    if (nextStatus === 'completed' && !form.getValues('watchedDate')) {
      form.setValue('watchedDate', localDate(now), { shouldDirty: true })
    }
  }

  return (
    <form className="quick-record-form" onSubmit={submit} noValidate>
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

      {status === 'completed' ? (
        <label className="form-field compact-field">
          <span>观看日期</span>
          <input type="date" {...form.register('watchedDate')} />
          {form.formState.errors.watchedDate ? <small>{form.formState.errors.watchedDate.message}</small> : null}
        </label>
      ) : null}

      <button className="expand-record-fields" type="button" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
        {expanded ? <ChevronUp aria-hidden="true" size={16} /> : <ChevronDown aria-hidden="true" size={16} />}
        {expanded ? '收起记录选项' : '更多记录选项'}
      </button>

      {expanded ? (
        <div className="optional-record-fields">
          <label className="form-field compact-field">
            <span>评分</span>
            <span className="rating-input">
              <input aria-label="评分" inputMode="decimal" type="number" min="0" max="10" step="0.1" {...form.register('rating')} />
              <span aria-hidden="true">/ 10</span>
            </span>
            {form.formState.errors.rating ? <small>{form.formState.errors.rating.message}</small> : null}
          </label>
          <label className="form-field compact-field">
            <span>观看方式</span>
            <input type="text" maxLength={80} placeholder="如：影院、家庭电视" {...form.register('viewingMethod')} />
          </label>
          {status === 'completed' && participants.length > 0 ? (
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

      {savedChange ? (
        <div className="save-toast" role="status">
          <span>记录已保存</span>
          {savedChange.undoable ? (
            <button
              type="button"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate({
                payload: savedChange.previous,
                version: savedChange.saved.version,
                action: 'undo',
              })}
            >
              撤销刚才的修改
            </button>
          ) : null}
        </div>
      ) : null}

      <div className="form-actions">
        <button className="primary-button" type="submit" disabled={mutation.isPending || status === 'none'}>
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
  )
}

type FormValues = {
  status: RecordStatus
  watchedDate: string
  rating: string
  note: string
  viewingMethod: string
  participantIds: string[]
}

type MutationVariables = {
  payload: UpdateRecordPayload
  version: number
  action: 'save' | 'undo'
}

const statusOptions = [
  { value: 'wishlist', label: '想看', icon: Bookmark },
  { value: 'watching', label: '在看', icon: Play },
  { value: 'completed', label: '看过', icon: Check },
  { value: 'dropped', label: '弃看', icon: CircleStop },
] satisfies Array<{ value: RecordStatus; label: string; icon: typeof Bookmark }>

const formSchema = z
  .object({
    status: z.enum(['none', 'wishlist', 'watching', 'completed', 'dropped']),
    watchedDate: z.string(),
    rating: z.string().refine((value) => value === '' || (!Number.isNaN(Number(value)) && Number(value) >= 0 && Number(value) <= 10), '评分必须在 0 到 10 之间'),
    note: z.string().max(5000, '笔记不能超过 5000 字'),
    viewingMethod: z.string().max(80, '观看方式不能超过 80 字'),
    participantIds: z.array(z.string().min(1)),
  })
  .superRefine((value, context) => {
    if (value.status === 'completed' && !value.watchedDate) {
      context.addIssue({ code: 'custom', path: ['watchedDate'], message: '请选择观看日期' })
    }
  })

function toPayload(values: FormValues, expanded: boolean): UpdateRecordPayload {
  const payload: UpdateRecordPayload = { status: values.status }
  if (values.status === 'completed' && values.watchedDate) payload.watchedAt = `${values.watchedDate}T12:00:00.000Z`
  if (values.status === 'completed' && values.participantIds.length > 0) payload.participantIds = values.participantIds
  if (expanded) {
    payload.rating = values.rating === '' ? null : Number(values.rating)
    payload.note = values.note.trim() || null
    payload.viewingMethod = values.viewingMethod.trim() || null
  }
  return payload
}

function undoPayload(record: RecordState, savedPayload: UpdateRecordPayload): UpdateRecordPayload {
  const payload: UpdateRecordPayload = { status: record.status }
  if ('rating' in savedPayload) payload.rating = record.rating
  if ('note' in savedPayload) payload.note = record.note
  return payload
}

function formValuesFromRecord(record: RecordState): FormValues {
  return {
    status: record.status,
    watchedDate: record.watchedAt?.slice(0, 10) ?? '',
    rating: record.rating === null ? '' : String(record.rating),
    note: record.note ?? '',
    viewingMethod: record.viewingMethod ?? '',
    participantIds: [],
  }
}

function localDate(value: Date) {
  const year = value.getUTCFullYear()
  const month = String(value.getUTCMonth() + 1).padStart(2, '0')
  const day = String(value.getUTCDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function parseETag(etag: string | null) {
  if (!etag) return null
  const version = Number(etag.replaceAll('"', ''))
  return Number.isInteger(version) && version >= 0 ? version : null
}
