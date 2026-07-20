<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Terminal as XTerm } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { TerminalWrite, TerminalResize } from '../../wailsjs/go/main/App'

  let { sessionId, onExit }: { sessionId: string; onExit: () => void } = $props()

  let container: HTMLDivElement
  let term: XTerm
  let fit: FitAddon
  let unsubs: Array<() => void> = []
  let resizeObserver: ResizeObserver

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
    TerminalResize(sessionId, term.cols, term.rows)

    unsubs.push(EventsOn(`terminal:data:${sessionId}`, (chunk: string) => term.write(chunk)))
    unsubs.push(EventsOn(`terminal:closed:${sessionId}`, () => onExit()))
    term.onData((data) => { TerminalWrite(sessionId, data) })

    resizeObserver = new ResizeObserver(() => {
      fit.fit()
      TerminalResize(sessionId, term.cols, term.rows)
    })
    resizeObserver.observe(container)
  })

  onDestroy(() => {
    for (const unsub of unsubs) unsub()
    resizeObserver?.disconnect()
    term?.dispose()
  })
</script>

<div class="term-pane" bind:this={container}></div>
