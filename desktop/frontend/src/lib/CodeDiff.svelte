<script lang="ts">
  // What one tool call changed, in git's own format.
  //
  // The engine builds the hunks (internal/skill/hunk.go) and hands them over on
  // the tool result; this reads them. Nothing here computes a diff, and that is
  // deliberate: the change belongs to the CALL, and by the time a row is
  // expanded the file on disk may have moved on twice.
  //
  // The classes and the --diff-* tokens are the Review panel's, removed on
  // 2026-08-03. Its visual language outlived it in style.css with nothing
  // drawing it. This is that language finding the room it belongs in: the โค้ด
  // desk, where someone is reading what an agent did to their code.
  import { t } from './i18n.svelte'

  let { diff }: { diff: string } = $props()

  type Row =
    | { kind: 'file'; text: string }
    | { kind: 'hunk'; text: string }
    | { kind: 'line'; sign: ' ' | '+' | '-'; no: number; text: string }
    | { kind: 'more'; count: number }

  const rows = $derived(parse(diff))

  const HUNK = /^@@ -(\d+),\d+ \+(\d+),\d+ @@/

  function parse(src: string): Row[] {
    const lines = src.split('\n')
    const out: Row[] = []
    let oldNo = 0
    let newNo = 0

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]

      // A file header only counts as one when a hunk follows it. An added line
      // reading "++ something" arrives here spelled exactly the same way, and
      // the difference between the two is what comes next.
      if (line.startsWith('+++ ') && HUNK.test(lines[i + 1] ?? '')) {
        out.push({ kind: 'file', text: line.slice(4) })
        continue
      }

      const hunk = HUNK.exec(line)
      if (hunk) {
        oldNo = Number(hunk[1])
        newNo = Number(hunk[2])
        out.push({ kind: 'hunk', text: line })
        continue
      }

      // The engine's one addition to git's format: the cut, said out loud.
      if (/^~\d+$/.test(line)) {
        out.push({ kind: 'more', count: Number(line.slice(1)) })
        continue
      }

      // Every remaining line is one line of the file, prefixed by what happened
      // to it. The number shown is the one that line HAS: a removed line is
      // only findable in the old file, everything else in the new one.
      const text = line.slice(1)
      if (line[0] === '+') {
        out.push({ kind: 'line', sign: '+', no: newNo++, text })
      } else if (line[0] === '-') {
        out.push({ kind: 'line', sign: '-', no: oldNo++, text })
      } else {
        out.push({ kind: 'line', sign: ' ', no: newNo++, text })
        oldNo++
      }
    }
    return out
  }
</script>

<div class="diff">
  {#each rows as row}
    {#if row.kind === 'file'}
      <div class="fname">{row.text}</div>
    {:else if row.kind === 'hunk'}
      <div class="hunk">{row.text}</div>
    {:else if row.kind === 'more'}
      <div class="dmore">{t('chat.diffMore', { n: row.count })}</div>
    {:else}
      <div class="dl" class:add={row.sign === '+'} class:del={row.sign === '-'}>
        <span class="ln">{row.no}</span>
        <span class="tx">{row.text || ' '}</span>
      </div>
    {/if}
  {/each}
</div>
