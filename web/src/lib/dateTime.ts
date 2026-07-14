function pad(value: number) {
	return String(value).padStart(2, '0')
}

export function formatLocalSeconds(value: string, timeZone?: string) {
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return ''
	const formatter = new Intl.DateTimeFormat('en-CA', {
		timeZone,
		year: 'numeric',
		month: '2-digit',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hourCycle: 'h23',
	})
	const parts = Object.fromEntries(
		formatter.formatToParts(date).map((part) => [part.type, part.value]),
	)
	return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}

export function toDateTimeLocalValue(date: Date) {
	if (Number.isNaN(date.getTime())) return ''
	return [
		`${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
		`${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`,
	].join('T')
}

export function fromDateTimeLocalValue(value: string) {
	const normalized = value.endsWith('.000') ? value.slice(0, -4) : value
	const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})$/.exec(normalized)
	if (!match) return null
	const [, yearValue, monthValue, dayValue, hourValue, minuteValue, secondValue] = match
	const values = [yearValue, monthValue, dayValue, hourValue, minuteValue, secondValue].map(Number)
	if (values.some((item) => !Number.isInteger(item))) return null
	const [year, month, day, hour, minute, second] = values as [number, number, number, number, number, number]
	const date = new Date(year, month - 1, day, hour, minute, second)
	if (toDateTimeLocalValue(date) !== normalized) return null
	return date
}

export function isFutureDateTimeLocalValue(value: string, now = new Date()) {
	const date = fromDateTimeLocalValue(value)
	return date !== null && date.getTime() > now.getTime()
}
