<script lang="ts">
  // The provider's own brand mark, for the provider list and the model picker.
  // Fifteen rows of identical text labels is the hardest kind of list to scan;
  // these are marks the user already recognises from everywhere else, so the
  // row they want is found by shape before the name is read.
  //
  // Path data lives in providerMarks.ts (generated — see the note there on the
  // source and on trademark). A provider with no mark falls back to a lettered
  // tile rather than a blank gap, so the row still has something to anchor on
  // and every row keeps the same width.
  import Logo from './Logo.svelte'
  import { PROVIDER_MARKS } from './providerMarks'

  let { name, size = 16 }: { name: string; size?: number } = $props()

  const mark = $derived(PROVIDER_MARKS[name] ?? '')
</script>

{#if name === 'aetox'}
  <!-- Aetox's own mark is a component, not a path here: it carries the brand
       colour and the entrance animation the others have no business having. -->
  <Logo {size} animate={false} />
{:else if mark}
  <!-- @html is safe: `mark` is only ever a value looked up out of the static
       generated map — `name` indexes it, it is never interpolated into markup. -->
  <svg
    class="pv-mark" width={size} height={size} viewBox="0 0 24 24"
    fill="currentColor" fill-rule="evenodd" aria-hidden="true"
  >{@html mark}</svg>
{:else}
  <span class="pv-mark pv-letter" style="--pv-size:{size}px" aria-hidden="true">
    {(name[0] ?? '?').toUpperCase()}
  </span>
{/if}
