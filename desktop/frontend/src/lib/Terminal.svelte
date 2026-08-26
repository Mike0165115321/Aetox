<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Terminal as XTerm } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { TerminalWrite, TerminalResize, TerminalAttach } from '../../wailsjs/go/main/App'
  import { isHostWebview } from './hostWebview'

  let { sessionId, onExit }: { sessionId: string; onExit: () => void } = $props()

  let container: HTMLDivElement
  let term: XTerm
  let fit: FitAddon
  let unsubs: Array<() => void> = []
  let resizeObserver: ResizeObserver

  // Same rule as BrowserPane (hostWebview.ts, §191): the PTY exists once, and
  // ConPTY replays the whole screen on every resize it receives - so a second
  // wails-dev frontend reporting ITS cols×rows would fight the app's over the
  // shell's wrap width, forever. A spectator still renders and may even type;
  // its own xterm wrapping at a width the PTY never heard of garbles only the
  // spectator's view, which is the right side to pay.
  const spectator = !isHostWebview()

  onMount(() => {
    term = new XTerm({
      convertEol: true,
      cursorBlink: true,
      fontFamily: 'ui-monospace, "Cascadia Code", "JetBrains Mono", Consolas, monospace',
      fontSize: 13,
      theme: { background: '#00000000' },
    })
    fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    fit.fit()
    if (!spectator) TerminalResize(sessionId, term.cols, term.rows)

    // Subscribe first, attach second. The session emits nothing until it is
    // attached, so this order is what guarantees the banner and prompt a shell
    // printed before this pane existed are neither lost nor shown twice — see
    // TerminalAttach. Reversing these two lines reopens the black-pane bug.
    unsubs.push(EventsOn(`terminal:data:${sessionId}`, (chunk: string) => term.write(chunk)))
    unsubs.push(EventsOn(`terminal:closed:${sessionId}`, () => onExit()))
    void TerminalAttach(sessionId).then((backlog) => {
      if (backlog) term.write(backlog)
    })
    term.onData((data) => { TerminalWrite(sessionId, data) })

    // Refitting is what the observer is for, and refitting is also what can
    // trigger it: fit() writes new dimensions into the DOM, a burst of output
    // makes a scrollbar appear and take a few pixels of width, and the observer
    // fires on that too. Unguarded the three chase each other for as long as
    // the output lasts, which is a terminal that flickers and stutters through
    // an entire server boot and settles the moment the boot stops.
    //
    // Coalesced to one fit per frame, and that is ALL that is done here.
    //
    // The first attempt also skipped telling the PTY when the fit came out the
    // same size, on the reasoning that a no-op resize is what restarts the
    // chase. Skipped HERE it broke: the shell wraps its own lines at the width
    // it was last told, so a width it never hears about leaves the pane
    // rendering one wrap column while the shell uses another — which is a
    // terminal that looks scrambled and, once the two disagree far enough,
    // stops responding (2026-08-12, reverted the same hour it shipped).
    //
    // So this side reports every fit, and the dedupe lives in Go instead
    // (TerminalResize, terminal.go), keyed on what the PTY actually received —
    // the one comparison that cannot skip a size the shell has not heard. It
    // must live somewhere: a ConPTY resize call replays the entire screen even
    // at unchanged dimensions, which is what smeared the desk terminal through
    // every engine boot the same day.
    resizeObserver = new ResizeObserver(() => {
      if (queued) return
      queued = requestAnimationFrame(() => {
        queued = 0
        fit.fit()
        if (!spectator) TerminalResize(sessionId, term.cols, term.rows)
      })
    })
    resizeObserver.observe(container)
  })

  onDestroy(() => {
    for (const unsub of unsubs) unsub()
    resizeObserver?.disconnect()
    if (queued) cancelAnimationFrame(queued)
    term?.dispose()
  })

  // Shared with onMount's observer so the pending frame can be cancelled on the
  // way out — a fit() running against a disposed terminal throws into the
  // console and leaves the pane looking broken for the next thing that opens.
  let queued = 0
</script>

<div class="term-pane" bind:this={container}></div>
