<script lang="ts">
  // One icon, drawn from the inlined Lucide set (see icons.ts).
  //
  // Always aria-hidden: an icon is never the accessible name. Icon-only buttons
  // carry their own aria-label, and an icon beside a label would otherwise be
  // read out twice.
  //
  // Size defaults to 16 — the size that sits level with body text. Stroke width
  // is scaled down slightly above 20px so a large icon does not read heavier
  // than the small ones next to it.
  import { ICONS, type IconName } from './icons'

  let { name, size = 16 }: { name: IconName; size?: number } = $props()

  const stroke = $derived(size > 20 ? 1.75 : 2)
</script>

<svg
  class="icon"
  width={size} height={size} viewBox="0 0 24 24"
  fill="none" stroke="currentColor" stroke-width={stroke}
  stroke-linecap="round" stroke-linejoin="round"
  aria-hidden="true"
>{@html ICONS[name]}</svg>
