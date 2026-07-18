import { useId } from 'react'

const MIN = 0
const MAX = 10
const STEP = 0.5

const steps = Array.from({ length: Math.round((MAX - MIN) / STEP) + 1 }, (_, index) =>
  Number((MIN + index * STEP).toFixed(1)),
)

type RatingPickerProps = {
  value: string
  onChange: (value: string) => void
  error?: string | undefined
  disabled?: boolean | undefined
}

export function RatingPicker({ value, onChange, error, disabled = false }: RatingPickerProps) {
  const baseID = useId()
  const labelID = `${baseID}-label`
  const errorID = `${baseID}-error`
  const numeric = parseRating(value)
  const display = numeric === null ? '未评分' : `${formatRating(numeric)} / 10`

  const setNumeric = (next: number | null) => {
    if (disabled) return
    onChange(next === null ? '' : formatRating(next))
  }

  const nudge = (direction: -1 | 1) => {
    const current = numeric ?? 0
    const next = clamp(roundToStep(current + direction * STEP))
    setNumeric(next)
  }

  return (
    <div className="rating-picker">
      <div className="rating-picker-heading">
        <span id={labelID}>评分</span>
        <strong aria-live="polite">{display}</strong>
      </div>
      <div
        className="rating-picker-scale"
        role="slider"
        tabIndex={disabled ? -1 : 0}
        aria-labelledby={labelID}
        aria-valuemin={MIN}
        aria-valuemax={MAX}
        aria-valuenow={numeric ?? undefined}
        aria-valuetext={display}
        aria-invalid={Boolean(error) || undefined}
        aria-describedby={error ? errorID : undefined}
        aria-disabled={disabled || undefined}
        onKeyDown={(event) => {
          if (disabled) return
          if (event.key === 'ArrowRight' || event.key === 'ArrowUp') {
            event.preventDefault()
            nudge(1)
          } else if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') {
            event.preventDefault()
            nudge(-1)
          } else if (event.key === 'Home') {
            event.preventDefault()
            setNumeric(MIN)
          } else if (event.key === 'End') {
            event.preventDefault()
            setNumeric(MAX)
          } else if (event.key === 'Delete' || event.key === 'Backspace') {
            event.preventDefault()
            setNumeric(null)
          }
        }}
      >
        {steps.map((step) => {
          const selected = numeric !== null && numeric >= step
          const active = numeric !== null && Math.abs(numeric - step) < 0.001
          return (
            <button
              key={step}
              type="button"
              className={`rating-step${selected ? ' selected' : ''}${active ? ' active' : ''}`}
              disabled={disabled}
              tabIndex={-1}
              aria-label={`评分 ${formatRating(step)}`}
              aria-pressed={active}
              onClick={() => setNumeric(active ? null : step)}
            >
              <span aria-hidden="true" />
            </button>
          )
        })}
      </div>
      <label className="rating-precise">
        <span>精确输入</span>
        <span className="rating-input">
          <input
            aria-label="精确评分"
            aria-invalid={Boolean(error) || undefined}
            aria-describedby={error ? errorID : undefined}
            inputMode="decimal"
            type="number"
            min={MIN}
            max={MAX}
            step={STEP}
            disabled={disabled}
            value={value}
            onChange={(event) => onChange(event.target.value)}
          />
          <span aria-hidden="true">/ 10</span>
        </span>
      </label>
      {error ? <small id={errorID}>{error}</small> : null}
    </div>
  )
}

export function parseRating(value: string): number | null {
  if (value.trim() === '') return null
  const number = Number(value)
  if (Number.isNaN(number)) return null
  return number
}

function formatRating(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function clamp(value: number) {
  return Math.min(MAX, Math.max(MIN, value))
}

function roundToStep(value: number) {
  return Math.round(value / STEP) * STEP
}
