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

  // How much of the project to draw, and how much of the drawing to spell out.
  //
  // Sixty was the right FIRST sentence and the wrong only one (owner, 31 ส.ค.:
  // "ทำให้เพดานมันปรับได้ไหม"). The ceiling is a question about this screen, not
  // about the repository, so it belongs on this screen — and the top of the
  // range is the project's own size rather than a number we invented, because
  // any invented number would be a second thing to keep true.
  //
  // Four settings, and each one buys back a cost that was measured, not
  // guessed: the count is layout time, the labels are the only thing that
  // actually turns a big map into felt, the lines are 1.1 per node of ink, and
  // the motion switch is the escape hatch when a drag on a 2,000-dot map costs
  // more than the drag is worth.
  // ALL_NODES is how the SURVEY is fetched, never a ceiling the reader picks:
  // the walk costs the same at any size (measured), so the whole ranked list
  // comes over once and the ceiling is applied to it here.
  //
  // 800 is the top of the range and 150 the opening one, both the owner's call
  // (31 ส.ค.). The machine would go further — 2,362 files lay out in 549ms —
  // but past a point the extra dots stop being a map of anything, and where
  // that point falls is a judgement about reading, not about milliseconds.
  const ALL_NODES = -1
  const MAX_NODES = 800
  const TIERS = [60, 150, 300, 500, MAX_NODES]
  const PREFS_KEY = 'aetox-repomap-display'
  type LabelMode = 'auto' | 'all' | 'off'

  let maxNodes = $state(150)
  let labelMode = $state<LabelMode>('auto')
  let showLines = $state(true)
  let motion = $state(true)
  let panelOpen = $state(false)
  let layoutMs = $state(0)

  function loadPrefs(): void {
    try {
      const p = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}')
      // Clamped on the way in, not just on the way out: a number remembered
      // from a build whose range went higher must not reopen above the range.
      if (typeof p.maxNodes === 'number' && p.maxNodes > 0) maxNodes = Math.min(MAX_NODES, Math.round(p.maxNodes))
      if (p.labelMode === 'auto' || p.labelMode === 'all' || p.labelMode === 'off') labelMode = p.labelMode
      if (typeof p.showLines === 'boolean') showLines = p.showLines
      if (typeof p.motion === 'boolean') motion = p.motion
    } catch {
      /* storage unavailable or corrupt — the defaults above are the answer */
    }
  }
  function savePrefs(): void {
    try {
      localStorage.setItem(PREFS_KEY, JSON.stringify({ maxNodes, labelMode, showLines, motion }))
    } catch {
      /* nothing to do: the map still works, it just forgets */
    }
  }
  // The staged reveal (owner, 29 ส.ค.: "ค่อยๆแสดงทีละตัวก็ได้ UX UI สำคัญมาก").
  // Nodes arrive in one answer but LIGHT UP one at a time, in rank order — the
  // load-bearing files first, satellites after — and an edge draws itself only
  // once both of its ends exist. Not a spinner wearing a costume: the order of
  // appearance is itself the map's first sentence about the project.
  let revealed = $state(0)
  let revealTimer: ReturnType<typeof setTimeout> | undefined

  // The paper the layout is drawn on. It GROWS with the node count, because a
  // fixed sheet is what turns more files into felt: the separation pass needs
  // roughly a 50-unit square per dot, so past about 280 dots a 1000x700 sheet
  // physically cannot hold them apart and they end up touching no matter how
  // many rounds it runs (measured: at 500 on the old fixed sheet the closest
  // pair sat 2.1 units INSIDE the gap it was owed; on a grown sheet it is 0).
  //
  // Density is therefore constant and the camera does the work instead —
  // fitCamera pulls back, dots get smaller on screen, and the wheel goes in.
  // That is what looking at more of something is supposed to cost.
  const W = 1000
  const H = 700
  let paperW = W
  let paperH = H

  function sizePaper(n: number): void {
    const k = Math.sqrt(Math.max(1, n) / 60)
    paperW = W * k
    paperH = H * k
  }
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

  // The survey, kept whole, and the ceiling applied to it afterwards.
  //
  // Walking the tree does NOT get cheaper by asking for fewer files: measured
  // on this repository, asking for 60 and asking for all 2,305 both cost about
  // 1.1 seconds, because the tree is walked and RANKED in full either way and
  // the ceiling only decides where the ranked list is cut. Re-surveying on
  // every change of the number was therefore paying a second and a bit for
  // information already in hand (owner, 31 ส.ค.: "บางทีมันโหลดใหม่ ไม่อยากให้
  // โหลดใหม่"). The walk now happens on open and on the refresh button, and
  // nowhere else; moving 100 to 200 is a relayout of what is already here.
  type RawNode = { path: string; dir: string; refs: number; symbols: number }
  let allNodes: RawNode[] = []
  let allEdges: GEdge[] = []

  async function load(): Promise<void> {
    loaded = false
    error = ''
    try {
      const g = await GetRepoMapGraph(ALL_NODES)
      focused = g.focused
      totalFiles = g.totalFiles ?? 0
      error = g.error ?? ''
      allNodes = (g.nodes ?? []) as RawNode[]
      allEdges = (g.edges ?? []) as GEdge[]
      draw()
    } catch (e) {
      error = String(e)
    }
    loaded = true
  }

  /** Cut the survey at the ceiling and lay that out. No walk, no Go call. */
  function draw(): void {
    const keep = Math.min(maxNodes, allNodes.length)
    const raw: GNode[] = allNodes.slice(0, keep).map((n) => ({ ...n, x: 0, y: 0, vx: 0, vy: 0 }))
    // Nodes arrive in rank order and the ceiling is a prefix of that order, so
    // an edge survives exactly when both of its ends did — the same cut Go
    // would have made, made here for free.
    const kept = allEdges.filter((e) => e.from < keep && e.to < keep)
    // Timed and shown in the panel, because a ceiling you can raise is a
    // ceiling whose price you are entitled to see before you raise it again.
    const began = performance.now()
    layout(raw, kept)
    layoutMs = performance.now() - began
    hover = -1
    nodes = raw
    edges = kept
    // Each edge remembers the distance the converged layout gave it, and the
    // live springs pull toward THAT — so dragging one file tugs its neighbours
    // along elastically while the shape the user has arranged stays the shape,
    // instead of one global physics reasserting itself.
    restLen = kept.map((e) => {
      const a = raw[e.from]
      const b = raw[e.to]
      // The floor matches the collision distance, or a spring and the
      // repulsion would tug the same pair in opposite directions forever.
      return a && b ? Math.max(72, Math.hypot(b.x - a.x, b.y - a.y)) : 90
    })
    fitCamera(raw)
    startReveal()
  }

  // About 1.3 seconds, whatever the count — long enough to read as the map
  // assembling itself, short enough never to be waited on.
  //
  // It used to be 22ms A DOT, which is the same 1.3 seconds at sixty and
  // eleven seconds at five hundred. The rhythm is the thing worth keeping, so
  // the tick stays 22ms and the CHUNK grows: the reveal is a fixed budget the
  // map spends, not a queue the viewer waits out.
  const REVEAL_TICK = 22
  const REVEAL_TICKS = 60

  function startReveal(): void {
    clearTimeout(revealTimer)
    if (!motion) {
      revealed = nodes.length
      return
    }
    revealed = 0
    const chunk = Math.max(1, Math.ceil(nodes.length / REVEAL_TICKS))
    const step = (): void => {
      if (revealed >= nodes.length) return
      revealed = Math.min(nodes.length, revealed + chunk)
      revealTimer = setTimeout(step, REVEAL_TICK)
    }
    step()
  }

  // Repulsion between files, springs along edges, gravity to the centre — the
  // standard trio, 260 rounds, then everything normalized to fill the paper.
  //
  // The repulsion runs through a Barnes-Hut quadtree rather than pushing every
  // pair: a clump of files far enough away pulls as one mass at its centre, so
  // the cost is n log n instead of n². Measured against the exact pass at 300
  // nodes the two pictures are the same picture — centroid within one unit,
  // closest pair within a third of one — and at 2,000 nodes it is the
  // difference between 1.9 seconds of frozen window and 0.6.
  //
  // Without this the adjustable ceiling would be a lie: an option the menu
  // offers and the window cannot survive is worse than no option.
  const THETA2 = 1.2 * 1.2

  function layout(ns: GNode[], es: GEdge[]): void {
    const n = ns.length
    if (n === 0) return
    sizePaper(n)
    for (let i = 0; i < n; i++) {
      const a = i * 2.39996
      const r = 14 * Math.sqrt(i + 0.5)
      ns[i].x = Math.cos(a) * r
      ns[i].y = Math.sin(a) * r
    }

    // Tree scratch, allocated once for the whole layout and rebuilt in place
    // each round. Flat typed arrays rather than objects: 260 rounds of garbage
    // is the kind of cost that only shows up on the machine you do not own.
    const cap = n * 8 + 256
    const massN = new Float64Array(cap)
    const sumX = new Float64Array(cap)
    const sumY = new Float64Array(cap)
    const kid = new Int32Array(cap * 4)
    const body = new Int32Array(cap)
    const boxX = new Float64Array(cap)
    const boxY = new Float64Array(cap)
    const boxS = new Float64Array(cap)
    const stack = new Int32Array(1024)
    let used = 0

    const clearCell = (c: number, x: number, y: number, s: number): void => {
      massN[c] = 0; sumX[c] = 0; sumY[c] = 0; body[c] = -1
      boxX[c] = x; boxY[c] = y; boxS[c] = s
      kid[c * 4] = -1; kid[c * 4 + 1] = -1; kid[c * 4 + 2] = -1; kid[c * 4 + 3] = -1
    }
    const quadrant = (c: number, i: number): number => {
      const half = boxS[c] / 2
      return (ns[i].x >= boxX[c] + half ? 1 : 0) + (ns[i].y >= boxY[c] + half ? 2 : 0)
    }
    const insert = (c: number, i: number, depth: number): void => {
      if (kid[c * 4] >= 0) {
        massN[c] += 1; sumX[c] += ns[i].x; sumY[c] += ns[i].y
        insert(kid[c * 4 + quadrant(c, i)], i, depth + 1)
        return
      }
      if (body[c] < 0) {
        body[c] = i; massN[c] = 1; sumX[c] = ns[i].x; sumY[c] = ns[i].y
        return
      }
      // Two files on one point never separate by subdividing, and the scratch
      // is finite — both are answered by giving up on the split and letting the
      // cell hold a small crowd. The separation pass below is what parts them.
      //
      // The crowd stops being "a cell holding body k" and becomes "a cell of
      // mass m centred here", which is the invariant the traversal reads: every
      // cell is either exactly one body or a mass with a centre. Leaving the
      // old body in place while adding to the mass would describe it as both,
      // and the traversal would then repel from one file while counting two.
      if (depth >= 40 || used + 4 > cap) {
        if (body[c] >= 0) {
          const alone = body[c]
          body[c] = -1
          massN[c] = 1; sumX[c] = ns[alone].x; sumY[c] = ns[alone].y
        }
        massN[c] += 1; sumX[c] += ns[i].x; sumY[c] += ns[i].y
        return
      }
      const old = body[c]
      const half = boxS[c] / 2
      for (let q = 0; q < 4; q++) {
        const child = used++
        kid[c * 4 + q] = child
        clearCell(child, boxX[c] + (q & 1) * half, boxY[c] + (q >> 1) * half, half)
      }
      body[c] = -1; massN[c] = 0; sumX[c] = 0; sumY[c] = 0
      insert(c, old, depth)
      insert(c, i, depth)
    }

    for (let round = 0; round < 260; round++) {
      const cool = 1 - round / 260
      let lo = Infinity, hi = -Infinity, loY = Infinity, hiY = -Infinity
      for (let i = 0; i < n; i++) {
        if (ns[i].x < lo) lo = ns[i].x
        if (ns[i].x > hi) hi = ns[i].x
        if (ns[i].y < loY) loY = ns[i].y
        if (ns[i].y > hiY) hiY = ns[i].y
      }
      used = 1
      clearCell(0, lo, loY, Math.max(hi - lo, hiY - loY) + 1)
      for (let i = 0; i < n; i++) insert(0, i, 0)

      for (let i = 0; i < n; i++) {
        let fx = -ns[i].x * 0.012
        let fy = -ns[i].y * 0.012
        let sp = 0
        stack[sp++] = 0
        while (sp > 0) {
          const c = stack[--sp]
          const m = massN[c]
          if (m === 0) continue
          const leaf = body[c]
          const bx = leaf >= 0 ? ns[leaf].x : sumX[c] / m
          const by = leaf >= 0 ? ns[leaf].y : sumY[c] / m
          const dx = ns[i].x - bx
          const dy = ns[i].y - by
          const d2 = dx * dx + dy * dy + 0.01
          if (leaf >= 0) {
            if (leaf === i) continue
            const push = 900 / d2
            fx += dx * push; fy += dy * push
          } else if (boxS[c] * boxS[c] / d2 < THETA2) {
            const push = 900 * m / d2
            fx += dx * push; fy += dy * push
          } else if (kid[c * 4] >= 0 && sp + 4 <= stack.length) {
            stack[sp++] = kid[c * 4]
            stack[sp++] = kid[c * 4 + 1]
            stack[sp++] = kid[c * 4 + 2]
            stack[sp++] = kid[c * 4 + 3]
          } else {
            const push = 900 * m / d2
            fx += dx * push; fy += dy * push
          }
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
    // Fit into the paper with margin, preserving aspect.
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity
    for (const p of ns) {
      minX = Math.min(minX, p.x); maxX = Math.max(maxX, p.x)
      minY = Math.min(minY, p.y); maxY = Math.max(maxY, p.y)
    }
    const pad = 70
    const s = Math.min((paperW - pad * 2) / Math.max(1, maxX - minX), (paperH - pad * 2) / Math.max(1, maxY - minY))
    for (const p of ns) {
      p.x = pad + (p.x - minX) * s + (paperW - pad * 2 - (maxX - minX) * s) / 2
      p.y = pad + (p.y - minY) * s + (paperH - pad * 2 - (maxY - minY) * s) / 2
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
    // Two changes to a pass that was already correct and quietly expensive.
    //
    // It stopped on "did anything move at all", which is true forever: the
    // pushes decay toward zero without ever reaching it, so every layout burned
    // all 220 rounds even when the picture had settled by round 13. Stopping at
    // a quarter of a unit — a third of the width of the thinnest line the pane
    // draws, on a gap that already carries 38 units of label room — costs
    // nothing anyone can see and returns 6 to 9 times the time: 60 nodes went
    // from 27ms to 3, and 500 from 1,002ms to 173.
    //
    // And it compared every pair to find the few that overlap. A uniform grid
    // of one gap-width cells asks only the 9 cells that could possibly hold a
    // toucher, which is what lets the top of the range exist at all.
    const rOf = (p: GNode): number => 4 + Math.min(11, Math.sqrt(p.refs) * 2.4)
    const gapFor = (a: GNode, b: GNode): number => rOf(a) + rOf(b) + 38
    let widest = 0
    for (const p of ns) widest = Math.max(widest, rOf(p))
    const cell = widest * 2 + 38
    const buckets = new Map<number, number[]>()
    for (let round = 0; round < 220; round++) {
      let baseX = Infinity, baseY = Infinity
      for (const p of ns) {
        if (p.x < baseX) baseX = p.x
        if (p.y < baseY) baseY = p.y
      }
      buckets.clear()
      for (let i = 0; i < n; i++) {
        const key = Math.floor((ns[i].x - baseX) / cell) * 100003 + Math.floor((ns[i].y - baseY) / cell)
        const b = buckets.get(key)
        if (b) b.push(i)
        else buckets.set(key, [i])
      }
      let biggest = 0
      for (let i = 0; i < n; i++) {
        const gx = Math.floor((ns[i].x - baseX) / cell)
        const gy = Math.floor((ns[i].y - baseY) / cell)
        for (let ox = -1; ox <= 1; ox++) {
          for (let oy = -1; oy <= 1; oy++) {
            const near = buckets.get((gx + ox) * 100003 + (gy + oy))
            if (!near) continue
            for (const j of near) {
              if (j <= i) continue
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
              const moved = dist * push
              if (moved > biggest) biggest = moved
              ns[i].x -= dx * push; ns[i].y -= dy * push
              ns[j].x += dx * push; ns[j].y += dy * push
            }
          }
        }
      }
      if (biggest <= 0.25) break
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
    // Bounds ride the paper, not the old fixed sheet: on a 2,000-file map a
    // ceiling of W*3 would stop the wheel before the map was fully in shot.
    const w = Math.min(paperW * 3, Math.max(paperW / 8, vb.w * f))
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

  // Who lights up with the hovered file, worked out ONCE per hover instead of
  // once per dot. The dim test used to rescan every edge for every node, which
  // is 4,000 comparisons at sixty dots and 275,000 at five hundred — on every
  // mouse move. The count was allowed to grow, so this had to stop growing
  // with it.
  const hoverKin = $derived.by(() => {
    const kin = new Set<number>()
    if (hover < 0) return kin
    kin.add(hover)
    for (const e of edges) {
      if (e.from === hover) kin.add(e.to)
      else if (e.to === hover) kin.add(e.from)
    }
    return kin
  })

  // Which dots get to say their name.
  //
  // Dots stay legible when you zoom out; NAMES do not, and 500 filenames at
  // 10px is the felt the ceiling was supposed to avoid. One rule, borrowed from
  // every paper map ever printed: name what there is room to name. Count what
  // is inside the camera right now, and if that is more than the budget, keep
  // the names of the files the project leans on hardest.
  //
  // The map behaviour falls out of it rather than being coded: zoom out and
  // only the hubs are labelled, zoom in and the small names arrive on their
  // own, because fewer of them are in shot.
  const LABEL_BUDGET = 80
  const named = $derived.by(() => {
    const show = new Set<number>()
    if (labelMode === 'off') return show
    const inView: number[] = []
    for (let i = 0; i < revealed && i < nodes.length; i++) {
      const p = nodes[i]
      if (p.x >= vb.x && p.x <= vb.x + vb.w && p.y >= vb.y && p.y <= vb.y + vb.h) inView.push(i)
    }
    if (labelMode === 'all' || inView.length <= LABEL_BUDGET) {
      for (const i of inView) show.add(i)
      return show
    }
    inView.sort((a, b) => nodes[b].refs - nodes[a].refs)
    for (let k = 0; k < LABEL_BUDGET; k++) show.add(inView[k])
    return show
  })

  /** A new ceiling on the survey we already have: remember it, redraw it. */
  function apply(): void {
    savePrefs()
    draw()
  }
  /** Settings that only change the drawing need no walk of the tree. */
  function applyDrawingOnly(): void {
    savePrefs()
    if (!motion && revealed < nodes.length) revealed = nodes.length
  }

  onMount(() => {
    loadPrefs()
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
      {#if showLines}
        {#each edges as e}
          {#if e.from < revealed && e.to < revealed}
            <line
              x1={nodes[e.from]?.x} y1={nodes[e.from]?.y}
              x2={nodes[e.to]?.x} y2={nodes[e.to]?.y}
              class="rm-edge grow" class:lit={hover >= 0 && touches(e, hover)}
            />
          {/if}
        {/each}
      {/if}
      {#each nodes.slice(0, revealed) as n, i}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <g
          class="rm-node pop" class:dim={hover >= 0 && !hoverKin.has(i)}
          style="transform-origin: {n.x}px {n.y}px"
          onmouseenter={() => (hover = i)} onmouseleave={() => (hover = -1)}
          onpointerdown={(e) => nodeDown(e, i)} onpointerup={() => up(i)}
        >
          <circle cx={n.x} cy={n.y} r={radius(n)} style="fill: hsl({coverHue(n.dir)} 45% 58%)" />
          {#if named.has(i)}
            <text x={n.x} y={n.y + radius(n) + 12}>{baseName(n.path)}</text>
          {/if}
          <title>{n.path} — {n.refs}×, {n.symbols} symbols</title>
        </g>
      {/each}
    </svg>
    <!-- The count in the sentence IS the handle for the panel that sets it, so
         the footer never states a number a second control could contradict. -->
    <div class="rm-foot">
      <span>{t('workbench.repoMapFiles', { shown: nodes.length, total: totalFiles })}</span>
      <div class="rm-foot-tools">
        <button
          class="icobtn tiny" class:on={panelOpen}
          aria-label={t('workbench.repoMapDisplay')} data-tip={t('workbench.repoMapDisplay')}
          aria-expanded={panelOpen}
          onclick={() => (panelOpen = !panelOpen)}
        >
          <Icon name="slidersHorizontal" size={13} />
        </button>
        <button class="icobtn tiny" aria-label={t('workbench.repoMapRefresh')} data-tip={t('workbench.repoMapRefresh')} onclick={() => void load()}>
          <Icon name="refreshCw" size={13} />
        </button>
      </div>

      {#if panelOpen}
        <div class="rm-panel">
          <!-- The count owns two lines, not one. Six choices and a typed number
               will not share a row with a label at this width: the first build
               pushed "ทั้งหมด" clean off the panel, which is the one choice a
               reader most needs to see is available. Label and box on top, the
               presets spanning the full width underneath. -->
          <div class="rm-row rm-row-stack">
            <div class="rm-row-head">
              <span>{t('workbench.repoMapCount')}</span>
              <input
                type="number" min="1" max={MAX_NODES} step="1"
                value={maxNodes}
                aria-label={t('workbench.repoMapCount')}
                onchange={(e) => {
                  const el = e.currentTarget as HTMLInputElement
                  const v = Math.round(Number(el.value))
                  // Clamped rather than refused: a typed 5000 becomes 800 and
                  // the box shows 800, so the map and the number always agree.
                  maxNodes = Number.isFinite(v) && v > 0 ? Math.min(MAX_NODES, v) : maxNodes
                  el.value = String(maxNodes)
                  apply()
                }}
              />
            </div>
            <div class="rm-seg rm-seg-fill">
              {#each TIERS as tier}
                <button
                  class:on={maxNodes === tier}
                  onclick={() => { maxNodes = tier; apply() }}
                >{tier}</button>
              {/each}
            </div>
          </div>

          <div class="rm-row">
            <span>{t('workbench.repoMapLabels')}</span>
            <div class="rm-seg">
              <button class:on={labelMode === 'auto'} onclick={() => { labelMode = 'auto'; applyDrawingOnly() }}>{t('workbench.repoMapLabelsAuto')}</button>
              <button class:on={labelMode === 'all'} onclick={() => { labelMode = 'all'; applyDrawingOnly() }}>{t('workbench.repoMapLabelsAll')}</button>
              <button class:on={labelMode === 'off'} onclick={() => { labelMode = 'off'; applyDrawingOnly() }}>{t('workbench.repoMapLabelsOff')}</button>
            </div>
          </div>

          <div class="rm-row">
            <span>{t('workbench.repoMapImportLines')}</span>
            <button
              class="rm-switch" class:on={showLines} role="switch" aria-checked={showLines}
              aria-label={t('workbench.repoMapImportLines')}
              onclick={() => { showLines = !showLines; applyDrawingOnly() }}
            ><span></span></button>
          </div>

          <div class="rm-row">
            <span>{t('workbench.repoMapMotion')}</span>
            <button
              class="rm-switch" class:on={motion} role="switch" aria-checked={motion}
              aria-label={t('workbench.repoMapMotion')}
              onclick={() => { motion = !motion; applyDrawingOnly() }}
            ><span></span></button>
          </div>

          <!-- What the current choice actually cost, measured on this machine
               on this project. A ceiling you can raise should show its bill. -->
          <p class="rm-cost">{t('workbench.repoMapCost', { ms: Math.round(layoutMs), named: named.size })}</p>
        </div>
      {/if}
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
    position: relative;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-2, 8px);
    padding: 6px 10px;
    border-top: 1px solid var(--border, rgba(127, 127, 127, 0.2));
    color: var(--text-muted);
    font-size: var(--fs-xs);
  }
  .rm-foot-tools {
    display: flex;
    align-items: center;
    gap: 2px;
    flex: none;
  }
  .rm-panel {
    position: absolute;
    right: 8px;
    bottom: calc(100% + 6px);
    z-index: 4;
    width: 296px;
    padding: 4px 12px 10px;
    border: 1px solid var(--border-default);
    border-radius: 10px;
    background: var(--surface-raised);
    box-shadow: var(--shadow-menu);
  }
  .rm-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 7px 0;
  }
  .rm-row + .rm-row {
    border-top: 1px solid var(--border-subtle);
  }
  .rm-row > span {
    white-space: nowrap;
  }
  .rm-row-stack {
    flex-direction: column;
    align-items: stretch;
    gap: 7px;
  }
  .rm-row-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  .rm-seg {
    display: flex;
    gap: 2px;
    padding: 2px;
    border-radius: 6px;
    background: var(--surface-sunken);
  }
  .rm-seg button {
    padding: 3px 7px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    cursor: pointer;
  }
  /* Six presets share the panel evenly instead of each taking what its own
     text needs, so the widest label cannot push the last one out of view.
     After the rule above, not before: same specificity, so order decides. */
  .rm-seg-fill button {
    flex: 1;
    min-width: 0;
    padding: 3px 2px;
    text-align: center;
  }
  .rm-seg button:hover {
    color: var(--text-primary);
  }
  .rm-seg button.on {
    background: var(--surface-hover);
    color: var(--text-primary);
  }
  .rm-row input[type='number'] {
    width: 78px;
    padding: 3px 6px;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--surface-sunken);
    color: var(--text-primary);
    font: inherit;
    text-align: right;
  }
  .rm-switch {
    flex: none;
    position: relative;
    width: 28px;
    height: 16px;
    padding: 0;
    border: 0;
    border-radius: 8px;
    background: var(--border-strong);
    cursor: pointer;
    transition: background 140ms ease;
  }
  .rm-switch.on {
    background: var(--accent);
  }
  .rm-switch span {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--text-on-interactive);
    transition: left 140ms ease;
  }
  .rm-switch.on span {
    left: 14px;
  }
  .rm-cost {
    margin: 8px 0 0;
    padding-top: 8px;
    border-top: 1px solid var(--border-subtle);
    color: var(--text-dim);
    font-size: var(--fs-xs);
  }
</style>
