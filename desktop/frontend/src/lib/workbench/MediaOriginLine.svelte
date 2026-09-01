<script lang="ts">
  // Where the file in this pane came from, under the thing it describes.
  //
  // Every word here is a number the editor reported or a label this app owns.
  // Nothing the agent WROTE reaches this strip, and that is the whole point of
  // it: "ตัดให้ 18 วิแล้วครับ" and a 40-second clip look identical in chat, and
  // stay identical right up until somebody watches the whole file. The line
  // says what the tool said it did, so the two can be compared at a glance.
  //
  // The operation keeps kinocut's own spelling — `trim`, `storyboard`,
  // `thumbnail` — set in the code face rather than translated. A Thai gloss
  // would be this app inventing a vocabulary for somebody else's fifty-four
  // tools and going stale on their next release; showing a machine word AS a
  // machine word is the honest presentation of it.
  //
  // Shared by MediaPane and ImagePane deliberately, the same way their headers
  // are: a file tab should feel like one room whatever is inside it, and a
  // storyboard is as much a result of a cut as the clip is.
  import { t } from '../i18n.svelte'
  import { mediaOrigin } from '../stores/workbench.svelte'

  let { path }: { path: string } = $props()

  const origin = $derived(mediaOrigin(path))

  // The tool's own word for what it did. `operation` when the result carried
  // one, otherwise the call's name with the server prefix taken off — which is
  // still kinocut's spelling, one step further out.
  const operation = $derived(origin?.operation || (origin?.tool ?? '').replace(/^kinocut_/, ''))

  /** mm:ss, or h:mm:ss once there is an hour to show. Seconds are floored:
   *  this labels a position on a bar, and a label reading 0:18 beside a mark
   *  drawn at 17.6 is the rounding lying about the drawing. */
  function clock(sec: number): string {
    const whole = Math.max(0, Math.floor(sec))
    const s = String(whole % 60).padStart(2, '0')
    const m = Math.floor(whole / 60) % 60
    const h = Math.floor(whole / 3600)
    return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
  }

  /** One decimal, and no trailing `.0`: 18.4 is a measurement, 18.0 is 18. */
  function trim1(n: number): string {
    return String(Math.round(n * 10) / 10)
  }

  const facts = $derived.by(() => {
    const o = origin
    if (!o || o.role !== 'result') return []
    const out: string[] = []
    if (o.duration) out.push(t('mediaOrigin.seconds', { n: trim1(o.duration) }))
    if (o.resolution) out.push(o.resolution)
    if (o.sizeMB) out.push(t('mediaOrigin.megabytes', { n: trim1(o.sizeMB) }))
    return out
  })

  // The bar is only ever drawn from a total Go was told, never one inferred
  // here (desktop/video_desk.go), so this needs no guard beyond "is it there".
  const plan = $derived(origin?.plan)
  const kept = $derived(plan ? plan.kept.reduce((sum, s) => sum + (s.end - s.start), 0) : 0)

  function pct(v: number, total: number): number {
    return total > 0 ? (v / total) * 100 : 0
  }
</script>

{#if origin}
  <div class="origin">
    {#if origin.role === 'source'}
      <span class="role">{t('mediaOrigin.source')}</span>
    {:else}
      <span class="role">{t('mediaOrigin.result')}</span>
      {#if operation}<code class="op">{operation}</code>{/if}
      {#each facts as fact (fact)}
        <span class="dot" aria-hidden="true">·</span><span class="fact">{fact}</span>
      {/each}
    {/if}
  </div>

  {#if plan && plan.total > 0}
    <div class="plan">
      <div class="bar">
        {#each plan.kept as span (span.start)}
          <div
            class="keep"
            style="left:{pct(span.start, plan.total)}%; width:{pct(span.end - span.start, plan.total)}%"
            title={span.label ?? `${clock(span.start)} - ${clock(span.end)}`}
          ></div>
        {/each}
        {#each plan.marks ?? [] as mark (mark.start)}
          <div class="mark" style="left:{pct(mark.start, plan.total)}%" title={clock(mark.start)}></div>
        {/each}
      </div>
      <div class="scale">
        <span>{clock(0)}</span>
        <span class="kept">{t('mediaOrigin.keptOf', { kept: clock(kept), total: clock(plan.total) })}</span>
        <span>{clock(plan.total)}</span>
      </div>
    </div>
  {/if}
{/if}

<style>
  /* One hairline above, none below: this strip belongs to the player it sits
     under, and a second rule would read as a separate panel. */
  .origin {
    flex: none;
    display: flex; align-items: baseline; flex-wrap: wrap; gap: 5px;
    padding: 6px 10px;
    border-top: 1px solid var(--border-default);
    font-size: var(--fs-xs); color: var(--text-muted);
  }
  .role { color: var(--text-secondary); }
  .op {
    font-family: var(--mono, monospace);
    background: var(--surface-sunken); border: 1px solid var(--border-default);
    border-radius: var(--r-sm); padding: 0 5px; color: var(--text-secondary);
  }
  .dot { opacity: 0.5; }

  .plan { flex: none; padding: 2px 10px 8px; }
  /* The dropped stretches are the track itself. Drawing them as a colour of
     their own would give "thrown away" the same weight as "kept", and the
     question this bar answers is which is which. */
  .bar {
    position: relative; height: 8px; border-radius: 999px;
    background: var(--surface-sunken); border: 1px solid var(--border-default);
    overflow: hidden;
  }
  .keep { position: absolute; top: 0; bottom: 0; background: var(--interactive); min-width: 2px; }
  /* Scene marks sit ON the track, not under it: they are positions in the same
     timeline, and a second row would invite reading them as a second measure. */
  .mark { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--surface-app); opacity: 0.85; }

  .scale {
    display: flex; justify-content: space-between; gap: 8px;
    margin-top: 4px; font-size: var(--fs-xs); color: var(--text-dim);
    font-variant-numeric: tabular-nums;
  }
  .scale .kept { color: var(--text-muted); }
</style>
