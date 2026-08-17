// Reading a motion token from the stylesheet, for the timings that a script
// owns rather than a transition.
//
// Most of Aetox's motion is CSS and takes its duration straight from a token
// (styles/type.css). A few are not: "say it worked, then move on" is a
// setTimeout, and so is the last screen of the first run closing itself. Those
// are the same kind of decision as a transition's length — how long a person is
// given to read that something happened — and writing the number a second time
// in TypeScript is how the two stop agreeing.
//
// So the token stays the only place the number lives, and this reads it.
// The fallback is not a second opinion: it is what to do when there is no
// stylesheet at all (a component under test in jsdom), where any number is as
// right as any other because nothing is on screen to watch.

/** Milliseconds for a `--dur-*` token. Accepts `850ms` or `.85s`. */
export function durationMs(token: string, fallback: number): number {
  try {
    const raw = getComputedStyle(document.documentElement).getPropertyValue(token).trim()
    if (raw.endsWith('ms')) {
      const n = parseFloat(raw)
      return Number.isFinite(n) ? n : fallback
    }
    if (raw.endsWith('s')) {
      const n = parseFloat(raw) * 1000
      return Number.isFinite(n) ? n : fallback
    }
  } catch {
    /* no stylesheet — fall through */
  }
  return fallback
}
