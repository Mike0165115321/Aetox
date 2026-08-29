import type { ToolStep } from './types'

/**
 * A turn, cut back into the stretches of work it was actually made of.
 *
 * The engine has recorded the sequence since §59 (internal/turn/part.go), and
 * the window then threw the shape away: every text part but the last became a
 * `note` row folded behind "ใช้ N เครื่องมือ", and both clocks were summed into
 * one number. A turn that thought for four minutes across four separate stops
 * arrived as "คิดเป็นเวลา 275 วินาที" over "ใช้ 36 เครื่องมือ · 272 วินาที",
 * which says the model did one enormous thing rather than nine, fourteen, eight
 * and five things with a sentence of its own in front of each.
 *
 * Nothing here is new information. 41+96+84+54 is the same 275, and the parts
 * that carry those four numbers have been in the database since migration v4.
 * This is the arithmetic stopping.
 */
export interface TurnPhase {
  /** The model's own words that opened this stretch. Empty on a phase that
   *  began with a tool call and no sentence in front of it. */
  say: string
  /** Seconds the model thought before saying it. Summed over the thinking rows
   *  that arrived since the previous phase, which is exactly the stretch this
   *  phase covers: a `thinking` row is emitted at the end of the round whose
   *  prose starts the phase after it. */
  thinkSecs: number
  /** Everything that ran under this stretch, in arrival order and still flat:
   *  a delegate's rows ride here beside the `task` row that hired it, so the
   *  caller's existing groupSteps/ownSteps/delegated helpers work unchanged. */
  steps: ToolStep[]
  /** This phase's prose is still arriving. Live turns only. */
  streaming: boolean
}

/**
 * Group an ordered step list into phases.
 *
 * The one rule: a sentence from the model starts a new phase. That is not a
 * heuristic about paragraphs, it is the shape of the protocol — a provider
 * streams prose, then the tool calls that prose announced, then more prose
 * (§59 puts the narration in front of the work it describes, and
 * executor_test.go pins the order as thinking → note → call → result).
 *
 * Used by both halves of the transcript, deliberately. The live block groups
 * `cockpit.toolSteps` as the events land; a reopened session groups what
 * stepsFromParts folds back out of the stored sequence. One function, because a
 * turn that is drawn one way while it runs and another way after a reload is
 * the app disagreeing with itself about what happened.
 *
 * @param steps  the flat, ordered list — live events or restored parts
 * @param streamingSay  prose still arriving for a round that has not closed.
 *   It becomes a trailing open phase, which is what stops the live view from
 *   erasing what the reader was already reading every time a tool starts.
 */
export function phasesOf(steps: ToolStep[], streamingSay = ''): TurnPhase[] {
  const phases: TurnPhase[] = []
  // Which phase a `task` row landed in, so its sub-agent's rows can join it
  // there. A delegate keeps working while the main agent narrates and calls
  // more tools, so its rows arrive interleaved with later phases — placed by
  // arrival they would be filed under a stretch of work they had nothing to do
  // with, under a sentence that was not about them.
  const homeOfRef = new Map<string, TurnPhase>()
  let pendingThink = 0
  let open: TurnPhase | null = null

  const start = (say: string, streaming: boolean): TurnPhase => {
    const phase: TurnPhase = { say, thinkSecs: pendingThink, steps: [], streaming }
    pendingThink = 0
    phases.push(phase)
    return phase
  }

  for (const step of steps) {
    if (step.parent) {
      const home = homeOfRef.get(step.parent) ?? open ?? (open = start('', false))
      home.steps.push(step)
      continue
    }
    if (step.kind === 'thinking') {
      pendingThink += step.secs ?? 0
      continue
    }
    // 'said' joins 'note' here rather than staying the separate thing it was.
    // The distinction existed because one was drawn as prose in the bubble and
    // the other as a muted row in a panel; with every phase drawing its prose
    // the same way, an answer an interjection re-placed is simply the phase it
    // always was. It keeps its markdown either way, which is the whole
    // complaint the Demoted flag was added to answer.
    if (step.kind === 'note' || step.kind === 'said') {
      open = start(step.label, false)
      continue
    }
    if (!open) open = start('', false)
    open.steps.push(step)
    if (step.ref) homeOfRef.set(step.ref, open)
  }

  if (streamingSay.trim()) {
    start(streamingSay, true)
  } else if (pendingThink > 0) {
    // Thought about, nothing said yet. Drawn as a header with no body rather
    // than added to the phase above it: that thinking happened after those
    // tools ran, and a stretch of silence is a true thing to show while the
    // next tool call is still being written.
    start('', false)
  }
  return phases
}
