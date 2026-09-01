<script lang="ts">
  // The face an เอเจน wears, drawn rather than stored.
  //
  // The mark this replaces was a glyph on a colour derived from the name
  // (coverHue), and the owner had already pushed it from a corner icon to the
  // agent's LOGO in the slot the eye lands on first. This carries that the rest
  // of the way: a small cartoon person, still derived from nothing but the name
  // and the `icon:` the profile already declares.
  //
  // Drawn, not a file, and that is the whole design. A portrait shipped as an
  // image would mean seven pieces of art for the seven bundled agents and
  // nothing at all for the .md a user drops in tomorrow — which would make
  // their own agent the only card in the roster wearing a blank. Here the same
  // rule that gives a new agent a colour gives it a face, in the same instant,
  // with nobody choosing one. profile.go's `icon:` comment refused a picture
  // path on the same reasoning and this does not overturn it: there is still no
  // second file to keep alive.
  //
  // Skin comes from the agent's own hue, so the cast is blue and green and
  // violet. That is deliberate rather than stylistic — a realistic portrait set
  // would mean choosing an apparent race and gender for seven employees, which
  // is a decision this app has no reason to make and no way to make well.
  //
  // This file is the FRAME. Every part lives in agentFace.ts, so adding a
  // haircut is appending one row there and nothing here changes.
  import { resolveFace, faceSVG, type FaceOverrides } from './agentFace'

  let {
    name,
    icon = undefined,
    size = 38,
    off = false,
    hue = undefined,
    hair = undefined,
    accessory = undefined,
  }: {
    name: string
    icon?: string
    size?: number
    off?: boolean
  } & FaceOverrides = $props()

  const face = $derived(resolveFace(name, size, { hue, hair, accessory, icon }))
  const inner = $derived(faceSVG(face))
</script>

<span
  class="agent-face"
  class:off
  style="--h:{face.hue}; width:{size}px; height:{size}px"
  aria-hidden="true"
>
  <!-- Our own markup out of agentFace.ts, never anything a user typed: the
       fields a profile may set choose a part by id, they do not carry one. -->
  <svg viewBox="0 0 64 64">{@html inner}</svg>
</span>

<style>
  /* The tile is the mark's, unchanged: same 26% corner, same two-stop ground,
     same inset hairline that costs no layout box. Only what sits on it is new,
     so a roster half-migrated does not read as two different galleries. */
  .agent-face {
    display: block;
    flex: none;
    overflow: hidden;
    border-radius: 26%;
    background: linear-gradient(158deg, hsl(var(--h) 44% 26%), hsl(var(--h) 40% 17%));
    box-shadow:
      inset 0 0 0 1px hsl(var(--h) 40% 38% / 0.55),
      inset 0 1px 0 hsl(var(--h) 60% 62% / 0.18);
  }
  /* Same drain the mark used for an agent the assistant may not hand work to.
     A face with its colour pulled out reads as out of service at a glance. */
  .agent-face.off {
    filter: saturate(0.2) brightness(0.72);
  }
  .agent-face svg {
    display: block;
    width: 100%;
    height: 100%;
  }
</style>
