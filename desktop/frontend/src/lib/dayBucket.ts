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

/** How many calendar days back this timestamp falls. Infinity when unreadable. */
export function daysAgo(iso: string | undefined): number {
  const parsed = iso ? Date.parse(iso) : NaN
  if (Number.isNaN(parsed)) return Infinity
  const todayStart = new Date()
  todayStart.setHours(0, 0, 0, 0)
  const thatDay = new Date(parsed)
  thatDay.setHours(0, 0, 0, 0)
  return Math.round((todayStart.getTime() - thatDay.getTime()) / DAY_MS)
}

// Split out of dayBucket rather than copied beside it, because the gallery now
// asks the same question a second way: "select everything from yesterday" has to
// mean the same yesterday the heading over those cards says. Two answers to
// which day it is, in one view, is a selection that quietly misses a card the
// user is looking straight at.
export function dayBucket(iso: string | undefined): TKey {
  const days = daysAgo(iso)
  if (!Number.isFinite(days)) return 'sidebar.older'
  if (days <= 0) return 'sidebar.today'
  if (days === 1) return 'sidebar.yesterday'
  if (days <= 7) return 'sidebar.last7Days'
  if (days <= 30) return 'sidebar.last30Days'
  return 'sidebar.older'
}
