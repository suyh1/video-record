import { Check, X } from 'lucide-react'
import { useId, useState, type RefObject } from 'react'

import {
  fromDateTimeLocalValue,
  isFutureDateTimeLocalValue,
  toDateTimeLocalValue,
} from '../../lib/dateTime'

type EpisodeTimeEditorProps = {
  episodeLabel: string
  watchedAt: string | null
  now: Date
  pending: boolean
  error?: string
  returnFocusRef?: RefObject<HTMLButtonElement | null>
  onConfirm: (watchedAt: string) => void
  onCancel: () => void
}

export function EpisodeTimeEditor({
  episodeLabel,
  watchedAt,
  now,
  pending,
  error = '',
  returnFocusRef,
  onConfirm,
  onCancel,
}: EpisodeTimeEditorProps) {
  const [value, setValue] = useState(() => watchedAt
    ? toDateTimeLocalValue(new Date(watchedAt))
    : toDateTimeLocalValue(now))
  const [validationError, setValidationError] = useState('')
  const errorID = useId()
  const visibleError = validationError || error

  const submit = (event: React.FormEvent) => {
    event.preventDefault()
    if (!value) {
      setValidationError('请选择观看时间')
      return
    }
    const parsed = fromDateTimeLocalValue(value)
    if (!parsed) {
      setValidationError('请输入有效的观看时间')
      return
    }
    if (isFutureDateTimeLocalValue(value, now)) {
      setValidationError('观看时间不能晚于当前时间')
      return
    }
    setValidationError('')
    onConfirm(parsed.toISOString())
  }

  const cancel = () => {
    onCancel()
    queueMicrotask(() => returnFocusRef?.current?.focus())
  }

  return (
    <form className="episode-time-editor" onSubmit={submit} noValidate>
      <label>
        <span>{episodeLabel} 观看时间</span>
        <input
          autoFocus
          type="datetime-local"
          step="1"
          max={toDateTimeLocalValue(now)}
          value={value}
          aria-invalid={Boolean(visibleError)}
          aria-describedby={visibleError ? errorID : undefined}
          disabled={pending}
          onChange={(event) => {
            setValue(event.target.value)
            setValidationError('')
          }}
        />
      </label>
      <div className="episode-time-actions">
        <button className="primary-button" type="submit" disabled={pending} aria-label={`确定 ${episodeLabel} 观看时间`}>
          <Check aria-hidden="true" size={15} />确定
        </button>
        <button type="button" disabled={pending} aria-label={`取消 ${episodeLabel} 观看时间`} onClick={cancel}>
          <X aria-hidden="true" size={15} />取消
        </button>
      </div>
      {visibleError ? <small id={errorID} role="alert">{visibleError}</small> : null}
    </form>
  )
}
