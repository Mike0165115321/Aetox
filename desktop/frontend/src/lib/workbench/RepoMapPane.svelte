<script lang="ts">
  // แผนที่โค้ด — the project as a constellation: dots joined by thin lines on
  // a quiet field. The data is repomap.Graph, the SAME analysis the
  // model's repo_map tool reads (desktop/repomap_view.go holds that promise),
  // so this pane is the human's window onto the map the model navigates by —
  // never a second opinion about the repository.
  //
  // The layout is a small force simulation run to convergence AT LOAD, then
  // yours: positions seed from a golden-angle spiral in rank order (no
  // randomness anywhere), so the same tree draws the same picture every time —
  // and then every dot can be dragged, the field panned, the wheel zoomed,
  // because a map you cannot rearrange is a poster (owner, 29 ส.ค.). A drag
  // moves a dot and nothing re-simulates: what you arranged stays arranged
  // until the next load.
  //
  // Clicking a dot opens the file HERE, as a workbench tab beside the map —
  // not in the chat column (owner's correction, same day): the map lives in
  // the AI's workbench, and what it opens should land on the same bench.
  import { onMount } from 'svelte'
  import { GetRepoMapGraph } from '../../../wailsjs/go/main/App'
  import { openFileTab } from '../stores/workbench.svelte'
  import { coverHue } from '../coverHue'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  type GNode = { path: string; dir: string; refs: number; symbols: number; x: number; y: number; vx: number; vy: number }
  type GEdge = { from: number; to: number }

  let nodes = $state<GNode[]>([])
  let edges = $state<GEdge[]>([])
  let totalFiles = $state(0)
  let focused = $state(true)
  let error = $state('')
  let loaded = $state(false)
  let hover = $state(-1)
  // The staged reveal (owner, 29 ส.ค.: "ค่อยๆแสดงทีละตัวก็ได้ UX UI สำคัญมาก").
  // Nodes arrive in one answer but LIGHT UP one at a time, in rank order — the
  // load-bearing files first, satellites after — and an edge draws itself only
  // once both of its ends exist. Not a spinner wearing a costume: the order of
  // appearance is itself the map's first sentence about the project.
  let revealed = $state(0)
  let revealTimer: ReturnType<typeof setTimeout> | undefined

  const W = 1000
  const H = 700
  // The camera: what part of the layout the svg shows. Zoom shrinks w/h, pan
  // moves x/y; the layout itself never changes under the camera.
  let vb = $state({ x: 0, y: 0, w: W, h: H })

  let svgEl: SVGSVGElement | undefined
  // One drag at a time: a node being carried, or the field being panned.
  let drag: { kind: 'node'; idx: number; moved: number } | { kind: 'pan'; sx: number; sy: number; ox: number; oy: number } | null = null

  // The live half of the physics (owner, 29 ส.ค.: "ลากอันนึงไปไกลๆ อันที่เกี่ยว
  // ต้องขยับไปด้วยดิ"). The layout() pass computes the resting picture once;
  // this loop is what makes the picture FLESH: springs along the edges tug a
  // dragged file's neighbours after it, a short-range push keeps dots off each
  // other, damping settles the wobble. It runs only while there is motion —
  // kick() wakes it, and when the energy dies it stops scheduling frames, so a
  // still map costs the machine nothing (the RAM/CPU line this product sells).
  let restLen: number[] = []
  let simRunning = false

  function kick(): void {
    if (simRunning) return
    simRunning = true
    requestAnimationFrame(simStep)
  }

  let dead = false

  function simStep(): void {
    const n = nodes.length
    if (dead || n === 0) {
      simRunning = false
      return
    }
    const pinned = drag?.kind === 'node' ? drag.idx : -1
    for (let k = 0; k < edges.length; k++) {
      const e = edges[k]
      const a = nodes[e.from]
      const b = nodes[e.to]
      if (!a || !b) continue
      const dx = b.x - a.x
      const dy = b.y - a.y
      const dist = Math.hypot(dx, dy) || 1
      const f = (dist - (restLen[k] ?? 90)) * 0.02
      const fx = (dx / dist) * f
      const fy = (dy / dist) * f
      a.vx += fx; a.vy += fy
      b.vx -= fx; b.vy -= fy
    }
    // Short-range only: full repulsion is layout()'s job and would fight the
    // user's arrangement — this just keeps dots from stacking. The range is
    // the same per-pair distance the first paint enforces, or a drag would
    // re-compress what the layout keeps apart.
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        const dx = nodes[i].x - nodes[j].x
        const dy = nodes[i].y - nodes[j].y
        const d2 = dx * dx + dy * dy
        const near = 8 + Math.min(11, Math.sqrt(nodes[i].refs) * 2.4) + 8 + Math.min(11, Math.sqrt(nodes[j].refs) * 2.4) + 30
        if (d2 > near * near || d2 === 0) continue
        const dist = Math.sqrt(d2)
        const push = ((near - dist) / dist) * 0.06
        nodes[i].vx += dx * push; nodes[i].vy += dy * push
        nodes[j].vx -= dx * push; nodes[j].vy -= dy * push
      }
    }
    let energy = 0
    for (let i = 0; i < n; i++) {
      const p = nodes[i]
      if (i === pinned) {
        p.vx = 0; p.vy = 0
        continue
      }
      p.vx *= 0.86; p.vy *= 0.86
      p.x += p.vx; p.y += p.vy
      energy += Math.abs(p.vx) + Math.abs(p.vy)
    }
    if (energy > 0.06 || pinned >= 0) {
      requestAnimationFrame(simStep)
    } else {
      simRunning = false
    }
  }

  async function load(): Promise<void> {
    loaded = false
    error = ''
    try {
      const g = await GetRepoMapGraph()
      focused = g.focused
      totalFiles = g.totalFiles ?? 0
      error = g.error ?? ''
      const raw = (g.nodes ?? []).map((n: { path: string; dir: string; refs: number; symbols: number }) => ({
        ...n, x: 0, y: 0, vx: 0, vy: 0,
      }))
      layout(raw, (g.edges ?? []) as GEdge[])
      nodes = raw
      edges = (g.edges ?? []) as GEdge[]
      // Each edge remembers the distance the converged layout gave it, and the
      // live springs pull toward THAT — so dragging one file tugs its
      // neighbours along elastically while the shape the user has arranged
      // stays the shape, instead of one global physics reasserting itself.
      restLen = edges.map((e) => {
        const a = raw[e.from]
        const b = raw[e.to]
        // The floor matches the collision distance, or a spring and the
        // repulsion would tug the same pair in opposite directions forever.
        return a && b ? Math.max(72, Math.hypot(b.x - a.x, b.y - a.y)) : 90
      })
      fitCamera(raw)
      startReveal()
    } catch (e) {
      error = String(e)
    }
    loaded = true
  }

  // ~22ms a dot: sixty dots settle in about 1.3 seconds — long enough to read
  // as the map assembling itself, short enough never to be waited on.
  function startReveal(): void {
    clearTimeout(revealTimer)
    revealed = 0
    const step = (): void => {
      if (revealed >= nodes.length) return
      revealed += 1
      revealTimer = setTimeout(step, 22)
    }
    step()
  }

  // Repulsion between every pair, springs along edges, gravity to the centre —
  // the standard trio, 260 rounds, then everything normalized to fill the
  // canvas. n is at most DefaultMaxNodes (60), so the O(n²) inner loop is
  // ~3,600 distance checks a round: milliseconds, once, at open.
  function layout(ns: GNode[], es: GEdge[]): void {
    const n = ns.length
    if (n === 0) return
    for (let i = 0; i < n; i++) {
      const a = i * 2.39996
      const r = 14 * Math.sqrt(i + 0.5)
      ns[i].x = Math.cos(a) * r
      ns[i].y = Math.sin(a) * r
    }
    for (let round = 0; round < 260; round++) {
      const cool = 1 - round / 260
      for (let i = 0; i < n; i++) {
        let fx = -ns[i].x * 0.012
        let fy = -ns[i].y * 0.012
        for (let j = 0; j < n; j++) {
          if (i === j) continue
          const dx = ns[i].x - ns[j].x
          const dy = ns[i].y - ns[j].y
          const d2 = dx * dx + dy * dy + 0.01
          const push = 900 / d2
          fx += dx * push
          fy += dy * push
        }
        ns[i].x += Math.max(-8, Math.min(8, fx)) * cool
        ns[i].y += Math.max(-8, Math.min(8, fy)) * cool
      }
      for (const e of es) {
        const a = ns[e.from]
        const b = ns[e.to]
        if (!a || !b) continue
        const dx = b.x - a.x
        const dy = b.y - a.y
        a.x += dx * 0.015 * cool
        a.y += dy * 0.015 * cool
        b.x -= dx * 0.015 * cool
        b.y -= dy * 0.015 * cool
      }
    }
    // Fit into the canvas with margin, preserving aspect.
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity
    for (const p of ns) {
      minX = Math.min(minX, p.x); maxX = Math.max(maxX, p.x)
      minY = Math.min(minY, p.y); maxY = Math.max(maxY, p.y)
    }
    const pad = 70
    const s = Math.min((W - pad * 2) / Math.max(1, maxX - minX), (H - pad * 2) / Math.max(1, maxY - minY))
    for (const p of ns) {
      p.x = pad + (p.x - minX) * s + (W - pad * 2 - (maxX - minX) * s) / 2
      p.y = pad + (p.y - minY) * s + (H - pad * 2 - (maxY - minY) * s) / 2
    }
    // Collision passes AFTER the fit, in screen units, run to done. The fit
    // compresses a wide layout, and compressed dots can land touching — which
    // the first paint then showed, and only a click (waking the live sim)
    // untangled (owner, 30 ส.ค.: "แสดงแบบสุ่มแล้วมันทับกันเอง ต้องกด 1 ที").
    // The first frame owes the viewer the untangled picture.
    //
    // The distance owed is per PAIR, not one constant: two hub dots are 15
    // units of radius each before their labels even start, and a flat gap
    // sized for small dots left the big ones shoulder-to-shoulder (owner,
    // same day: "ลูกบอลมันชิดกันไป"). Radii plus label breathing room.
    const rOf = (p: GNode): number => 4 + Math.min(11, Math.sqrt(p.refs) * 2.4)
    const gapFor = (a: GNode, b: GNode): number => rOf(a) + rOf(b) + 38
    for (let round = 0; round < 220; round++) {
      let moved = false
      for (let i = 0; i < n; i++) {
        for (let j = i + 1; j < n; j++) {
          let dx = ns[j].x - ns[i].x
          let dy = ns[j].y - ns[i].y
          let dist = Math.hypot(dx, dy)
          const minGap = gapFor(ns[i], ns[j])
          if (dist >= minGap) continue
          if (dist < 0.01) {
            // Two dots on one point have no direction to part in — give them
            // one from their indices, so the untangle stays deterministic.
            const a = (i * 7 + j) % 12 / 12 * Math.PI * 2
            dx = Math.cos(a); dy = Math.sin(a); dist = 1
          }
          const push = (minGap - dist) / dist / 2
          ns[i].x -= dx * push; ns[i].y -= dy * push
          ns[j].x += dx * push; ns[j].y += dy * push
          moved = true
        }
      }
      if (!moved) break
    }
    // No clamping back into the canvas: separation is allowed to spill past
    // it, and the CAMERA goes to the content instead (fitCamera) — walls that
    // shove border nodes together would undo at the edges exactly what the
    // passes above did in the middle.
  }

  /** Point the camera at everything, with margin — the opening shot. */
  function fitCamera(ns: GNode[]): void {
    if (ns.length === 0) {
      vb = { x: 0, y: 0, w: W, h: H }
      return
    }
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity
    for (const p of ns) {
      minX = Math.min(minX, p.x); maxX = Math.max(maxX, p.x)
      minY = Math.min(minY, p.y); maxY = Math.max(maxY, p.y)
    }
    const m = 70
    vb = { x: minX - m, y: minY - m, w: Math.max(1, maxX - minX) + m * 2, h: Math.max(1, maxY - minY) + m * 2 }
  }

  /** Pointer position in layout coordinates, camera applied. */
  function toWorld(e: PointerEvent | WheelEvent): { x: number; y: number } {
    const r = svgEl!.getBoundingClientRect()
    // The svg letterboxes (preserveAspectRatio meet): the drawn area is the
    // largest vb-shaped box that fits, centred. Mapping through the box, not
    // the element, or a drag away from the centre lags the cursor.
    const s = Math.min(r.width / vb.w, r.height / vb.h)
    const dx = (r.width - vb.w * s) / 2
    const dy = (r.height - vb.h * s) / 2
    return { x: vb.x + (e.clientX - r.left - dx) / s, y: vb.y + (e.clientY - r.top - dy) / s }
  }

  function nodeDown(e: PointerEvent, idx: number): void {
    e.stopPropagation()
    ;(e.currentTarget as Element).setPointerCapture(e.pointerId)
    drag = { kind: 'node', idx, moved: 0 }
    kick()
  }
  function fieldDown(e: PointerEvent): void {
    ;(e.currentTarget as Element).setPointerCapture(e.pointerId)
    drag = { kind: 'pan', sx: e.clientX, sy: e.clientY, ox: vb.x, oy: vb.y }
  }
  function move(e: PointerEvent): void {
    if (!drag || !svgEl) return
    if (drag.kind === 'node') {
      const p = toWorld(e)
      const n = nodes[drag.idx]
      drag.moved += Math.abs(p.x - n.x) + Math.abs(p.y - n.y)
      n.x = p.x
      n.y = p.y
      kick()
    } else {
      const r = svgEl.getBoundingClientRect()
      const s = Math.min(r.width / vb.w, r.height / vb.h)
      vb.x = drag.ox - (e.clientX - drag.sx) / s
      vb.y = drag.oy - (e.clientY - drag.sy) / s
    }
  }
  function up(idx?: number): void {
    // A press that barely moved is a click, and a click means "open it" — as a
    // tab on this bench, beside the map, never in the chat column.
    if (drag?.kind === 'node' && drag.moved < 5 && idx !== undefined) {
      void openFileTab(nodes[idx].path, baseName(nodes[idx].path))
    }
    drag = null
    // The release is when the elastic lets go: neighbours the drag stretched
    // are still under tension and settle over the next moments.
    kick()
  }
  function wheel(e: WheelEvent): void {
    if (!svgEl) return
    e.preventDefault()
    const p = toWorld(e)
    const f = e.deltaY > 0 ? 1.12 : 1 / 1.12
    const w = Math.min(W * 3, Math.max(W / 8, vb.w * f))
    const scale = w / vb.w
    vb = { x: p.x - (p.x - vb.x) * scale, y: p.y - (p.y - vb.y) * scale, w, h: vb.h * scale }
  }

  function radius(n: GNode): number {
    return 4 + Math.min(11, Math.sqrt(n.refs) * 2.4)
  }
  function baseName(path: string): string {
    return path.split('/').pop() ?? path
  }
  function touches(e: GEdge, i: number): boolean {
    return e.from === i || e.to === i
  }

  onMount(() => {
    void load()
    return () => {
      dead = true
      clearTimeout(revealTimer)
    }
  })
</script>

<div class="repomap-pane">
  {#if !loaded}
    <div class="rm-note rm-wait">
      <span class="rm-pulse"></span>
      {t('workbench.repoMapLoading')}
    </div>
  {:else if !focused}
    <div class="rm-note">{t('workbench.repoMapEmpty')}</div>
  {:else if error}
    <div class="rm-note">{error}</div>
  {:else}
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <svg
      bind:this={svgEl}
      viewBox="{vb.x} {vb.y} {vb.w} {vb.h}" preserveAspectRatio="xMidYMid meet"
      role="img" aria-label={t('workbench.repoMapTab')}
      onpointerdown={fieldDown} onpointermove={move} onpointerup={() => up()} onwheel={wheel}
    >
      {#each edges as e}
        {#if e.from < revealed && e.to < revealed}
          <line
            x1={nodes[e.from]?.x} y1={nodes[e.from]?.y}
            x2={nodes[e.to]?.x} y2={nodes[e.to]?.y}
            class="rm-edge grow" class:lit={hover >= 0 && touches(e, hover)}
          />
        {/if}
      {/each}
      {#each nodes.slice(0, revealed) as n, i}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <g
          class="rm-node pop" class:dim={hover >= 0 && hover !== i && !edges.some((e) => touches(e, hover) && touches(e, i))}
          style="transform-origin: {n.x}px {n.y}px"
          onmouseenter={() => (hover = i)} onmouseleave={() => (hover = -1)}
          onpointerdown={(e) => nodeDown(e, i)} onpointerup={() => up(i)}
        >
          <circle cx={n.x} cy={n.y} r={radius(n)} style="fill: hsl({coverHue(n.dir)} 45% 58%)" />
          <text x={n.x} y={n.y + radius(n) + 12}>{baseName(n.path)}</text>
          <title>{n.path} — {n.refs}×, {n.symbols} symbols</title>
        </g>
      {/each}
    </svg>
    <div class="rm-foot">
      <span>{t('workbench.repoMapFiles', { shown: nodes.length, total: totalFiles })}</span>
      <button class="icobtn tiny" aria-label={t('workbench.repoMapRefresh')} data-tip={t('workbench.repoMapRefresh')} onclick={() => void load()}>
        <Icon name="refreshCw" size={13} />
      </button>
    </div>
  {/if}
</div>

<style>
  .repomap-pane {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
  }
  .repomap-pane svg {
    flex: 1;
    min-height: 0;
    width: 100%;
    cursor: grab;
    touch-action: none;
  }
  .repomap-pane svg:active {
    cursor: grabbing;
  }
  .rm-edge {
    stroke: var(--text-muted);
    stroke-width: 1.1;
    opacity: 0.35;
    transition: opacity 120ms ease;
    pointer-events: none;
  }
  .rm-edge.lit {
    opacity: 0.9;
  }
  .rm-node {
    cursor: pointer;
    transition: opacity 120ms ease;
  }
  .rm-node.dim {
    opacity: 0.25;
  }
  .rm-node circle {
    stroke: var(--surface-1, transparent);
    stroke-width: 1.5;
  }
  .rm-node text {
    font-size: 10px;
    fill: var(--text-muted);
    text-anchor: middle;
    user-select: none;
  }
  .rm-note {
    margin: auto;
    color: var(--text-muted);
    font-size: var(--fs-sm);
  }
  .rm-wait {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  /* The wait is a dot of the map breathing, not a spinner: the first pixel the
     pane shows is already the material the answer will be made of. */
  .rm-pulse {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: rm-breathe 1.1s ease-in-out infinite;
  }
  @keyframes rm-breathe {
    0%, 100% { transform: scale(0.7); opacity: 0.4; }
    50% { transform: scale(1.15); opacity: 0.9; }
  }
  .rm-node.pop {
    animation: rm-pop 260ms ease-out;
  }
  @keyframes rm-pop {
    from { transform: scale(0); opacity: 0; }
    to { transform: scale(1); opacity: 1; }
  }
  .rm-edge.grow {
    animation: rm-fade 300ms ease-out;
  }
  @keyframes rm-fade {
    from { opacity: 0; }
  }
  .rm-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-2, 8px);
    padding: 6px 10px;
    border-top: 1px solid var(--border, rgba(127, 127, 127, 0.2));
    color: var(--text-muted);
    font-size: var(--fs-xs);
  }
</style>
