// Which day a timestamp belongs to, as a translation key.
//
// "3 วันที่แล้ว" on fourteen rows in a column is not a list, it is a wall. The
// ago-label answers "how long ago"; this answers "which day" — and only the
// second one lets the eye skip. Calendar days, not 24-hour buckets: a chat from
// 11pm last night is yesterday's at 1am, whatever the arithmetic says.
//
// It lives here rather than in the sidebar because the chat history and the
// office's job feed are the same question asked twice, and two copies of this
// arithmetic would drift the day two lists disagree about what "เมื่อวาน" is.
import type { TKey } from './i18n.svelte'

const DAY_MS = 86_400_000

export function dayBucket(iso: string | undefined): TKey {
  const parsed = iso ? Date.parse(iso) : NaN
  if (Number.isNaN(parsed)) return 'sidebar.older'
  const todayStart = new Date()
  todayStart.setHours(0, 0, 0, 0)
  const thatDay = new Date(parsed)
  thatDay.setHours(0, 0, 0, 0)
  const days = Math.round((todayStart.getTime() - thatDay.getTime()) / DAY_MS)
  if (days <= 0) return 'sidebar.today'
  if (days === 1) return 'sidebar.yesterday'
  if (days <= 7) return 'sidebar.last7Days'
  if (days <= 30) return 'sidebar.last30Days'
  return 'sidebar.older'
}
