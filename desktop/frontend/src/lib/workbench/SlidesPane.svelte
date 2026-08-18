<script lang="ts">
  // เด็คบนโต๊ะ อ่านเป็นสไลด์ ไม่ใช่เป็นซอร์ส
  //
  // ไม่มีอะไรในนี้เรนเดอร์เด็ค — ไฟล์คือ HTML และเว็บวิวคือเครื่องเรนเดอร์ HTML
  // ที่แอปนี้พกมาอยู่แล้ว หน้าที่ของแพเนลคือหาเส้นแบ่งสไลด์แล้วเดินไปทีละหน้า
  // เท่านั้น ดีไซน์ทั้งหมดเป็นของ CSS ในไฟล์ ซึ่งเป็นเหตุผลที่ย้ายมาเป็น HTML
  // ตั้งแต่แรก ถ้าแพเนลบังคับหน้าตาเองก็เท่ากับสร้าง PowerPoint ขึ้นมาใหม่ในที่
  // ที่ยากกว่าเดิม
  //
  // เข้าถึง DOM ในไอเฟรมได้เพราะไฟล์โฮสต์เสิร์ฟที่ /aetox-file/... ซึ่งเป็น
  // same-origin กับหน้าต่างนี้ (fileUrl.ts) จึงไม่ต้องฉีดสคริปต์ลงไฟล์ของผู้ใช้
  // เพื่อสั่งให้มันเลื่อน — ไฟล์เด็คยังเป็น HTML ที่เปิดในเบราว์เซอร์ไหนก็ได้
  // และไม่มีร่องรอยของแอปนี้ติดอยู่ในนั้น
  import { onMount } from 'svelte'
  import { DeckFormats, ExportDeck, OpenExport, OpenFileExternally } from '../../../wailsjs/go/main/App'
  import type { main } from '../../../wailsjs/go/models'
  import FileEditor from '../FileEditor.svelte'
  import { fileURL } from '../fileUrl'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  let { path, name, content }: { path: string; name: string; content: string } = $props()

  const src = $derived(fileURL(path))

  let frame = $state<HTMLIFrameElement | null>(null)
  let stage = $state<HTMLDivElement | null>(null)
  let slides = $state<HTMLElement[]>([])
  let current = $state(0)
  let source = $state(false)
  let failure = $state('')

  // อ่านรายการสไลด์ใหม่ทุกครั้งที่ไอเฟรมโหลดเสร็จ ไม่ใช่ครั้งเดียวตอน mount
  // เพราะเอเจนต์เขียนทับไฟล์เดิมบ่อยมาก และ Workbench คีย์แพเนลไว้ที่ rev อยู่
  // แล้ว การอ่านซ้ำตรงนี้กันกรณีที่ไอเฟรมเองนำทางใหม่โดยที่แท็บไม่ถูกอ่านซ้ำ
  function scan() {
    failure = ''
    const doc = frameDoc()
    if (!doc) return
    slides = Array.from(doc.querySelectorAll<HTMLElement>('section.slide'))
    current = 0
    doc.addEventListener('keydown', onKey)
    doc.addEventListener('scroll', syncFromScroll, { passive: true, capture: true })
    fit()
    // อีกครั้งหลังเฟรมถัดไป เพราะฟอนต์กับรูปอาจยังจัดหน้าไม่นิ่งตอน load ยิง
    // ซึ่งเป็นเหตุผลที่อาการ "บางทีก็เพี้ยน" เพี้ยนไม่เท่ากันทุกครั้ง
    requestAnimationFrame(fit)
  }

  // ย่อเด็คให้พอดีแพเนล
  //
  // สัญญาโครงตรึงสไลด์ไว้ที่ 1280 × 720 แล้วบอกว่า "แพเนลย่อให้พอดีจอจากมัน"
  // ตอนแรกผมเขียนแค่ครึ่งแรก ไอเฟรมเลยแสดงเด็คขนาดจริงในแพเนลที่แคบกว่า ผลคือ
  // ขอบขวาโดนตัด และสไลด์ถัดไปโผล่ขึ้นมาจากข้างล่าง
  //
  // ใช้ `zoom` ไม่ใช่ `transform: scale()` เพราะ zoom จัดหน้าใหม่จริง — ระยะเลื่อน
  // กับ getBoundingClientRect จึงเป็นค่าหลังย่อ ซึ่งคือค่าที่ goto กับ
  // syncFromScroll ต้องใช้ ส่วน transform หลอกแค่ตอนวาด ตัวเลขที่อ่านได้จะยัง
  // เป็นของขนาดเดิม แล้วการเลื่อนจะเพี้ยนขึ้นเรื่อย ๆ ทีละสไลด์
  //
  // เขียนลง documentElement ของไฟล์ที่โหลดอยู่ ไม่ใช่ลงไฟล์ — เด็คบนดิสก์ไม่ถูก
  // แตะ และยังเปิดในเบราว์เซอร์อื่นได้เหมือนเดิม
  function fit() {
    const doc = frameDoc()
    const box = stage?.getBoundingClientRect()
    if (!doc || !box || slides.length === 0 || box.width === 0) return

    const el = doc.documentElement as HTMLElement
    el.style.zoom = '1' // วัดขนาดจริงก่อน ไม่ใช่ขนาดที่ย่อไว้รอบก่อน
    const slide = slides[0].getBoundingClientRect()
    if (slide.width === 0 || slide.height === 0) return

    // พอดีทั้งใบ ไม่ใช่พอดีแค่ด้านกว้าง สไลด์หนึ่งใบต้องอยู่ในสายตาทั้งใบ
    // ไม่งั้นคนดูต้องเลื่อนเพื่ออ่านบรรทัดสุดท้ายของสไลด์ที่ควรเห็นทีเดียวจบ
    // ไม่ขยายเกิน 1 เพราะเด็คถูกออกแบบมาที่ขนาดของมัน การขยายทำให้ฟอนต์บวม
    const scale = Math.min(box.width / slide.width, box.height / slide.height, 1)
    el.style.zoom = String(scale)
    // ย่อแล้วตำแหน่งเลื่อนเดิมชี้ไปคนละที่ จึงต้องพากลับมาที่สไลด์เดิม
    slides[current]?.scrollIntoView({ block: 'center' })
  }

  // contentDocument โยนได้ถ้าเอกสารในไอเฟรมกลายเป็นคนละ origin ซึ่งเกิดได้ถ้า
  // เด็คมีลิงก์ที่พาตัวเองออกไปข้างนอก จับไว้เพื่อให้แพเนลเงียบแทนที่จะพัง
  function frameDoc(): Document | null {
    try {
      return frame?.contentDocument ?? null
    } catch {
      return null
    }
  }

  function goto(i: number) {
    if (slides.length === 0) return
    current = Math.max(0, Math.min(slides.length - 1, i))
    slides[current]?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }

  // ผู้ใช้เลื่อนเองได้ ตัวนับจึงต้องตามการเลื่อนจริง ไม่ใช่ตามปุ่มที่กดล่าสุด
  // เลือกสไลด์ที่ขอบบนอยู่ใกล้ขอบบนจอที่สุด แทนที่จะหารด้วยความสูง เพราะระยะ
  // ห่างระหว่างสไลด์เป็นของ CSS ในไฟล์ ซึ่งแพเนลไม่ควรรู้
  function syncFromScroll() {
    if (slides.length === 0) return
    let best = 0
    let nearest = Infinity
    const viewMiddle = (frameDoc()?.documentElement.clientHeight ?? 0) / 2
    for (let i = 0; i < slides.length; i++) {
      const r = slides[i].getBoundingClientRect()
      const gap = Math.abs(r.top + r.height / 2 - viewMiddle)
      if (gap < nearest) {
        nearest = gap
        best = i
      }
    }
    current = best
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowRight' || e.key === 'PageDown' || e.key === ' ') {
      e.preventDefault()
      goto(current + 1)
    } else if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
      e.preventDefault()
      goto(current - 1)
    } else if (e.key === 'Home') {
      e.preventDefault()
      goto(0)
    } else if (e.key === 'End') {
      e.preventDefault()
      goto(slides.length - 1)
    }
  }

  // เต็มจอเฉพาะกล่องเวที ไม่ใช่ทั้งหน้าต่าง เพราะสิ่งที่คนดูควรเห็นคือสไลด์
  // ไม่ใช่แถบเครื่องมือของแอปที่บังเอิญอยู่รอบ ๆ
  async function present() {
    failure = ''
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await stage?.requestFullscreen()
    } catch (err) {
      failure = String(err)
    }
  }

  // ส่งออก
  //
  // เมนูไม่ได้เก็บรายการฟอร์แมตของตัวเอง มันถามฝั่ง Go (DeckFormats) เพราะ
  // "เขียนฟอร์แมตนี้ได้ไหม" เป็นข้อเท็จจริงเกี่ยวกับไบนารี ไม่ใช่เกี่ยวกับปุ่ม
  // วันที่ PrintToPdf ลง แถว .pdf จะใช้งานได้เองโดยไม่ต้องแก้ไฟล์นี้ และไม่มี
  // ช่วงเวลาไหนที่สองรายการไม่ตรงกัน
  let formats = $state<main.DeckFormat[]>([])
  let menuOpen = $state(false)
  let busy = $state(false)
  let landed = $state('')

  // แพเนลเปลี่ยนขนาดได้ตลอด — ลากขอบ ย่อขยายหน้าต่าง เข้าออกเต็มจอ อัตราส่วนที่
  // ย่อไว้ตอนโหลดจึงหมดอายุทันที
  //
  // ตรวจว่ามี ResizeObserver ก่อน เพราะ jsdom ไม่มี และแพเนลที่โยน ReferenceError
  // ตอน mount คือแพเนลที่ทดสอบไม่ได้เลย ทั้งที่ตัวมันเองไม่ได้พึ่งการย่อ
  $effect(() => {
    if (!stage || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => fit())
    ro.observe(stage)
    return () => ro.disconnect()
  })

  onMount(async () => {
    try {
      formats = await DeckFormats()
    } catch {
      // เมนูที่โหลดไม่ได้กลายเป็นเมนูว่าง ซึ่งดีกว่าปุ่มที่เดารายการเอง
    }
  })

  // คำต่อท้ายของแถวที่นามสกุลอย่างเดียวบอกไม่พอ ฟอร์แมตที่ไม่มีคำต่อท้ายก็ไม่ต้องมี
  //
  // เขียนเป็นคีย์ตรง ๆ ไม่ใช่ตารางที่อินเด็กซ์ด้วย string เพราะ t() รับเฉพาะคีย์ที่
  // มีจริง — ตารางจะทำให้คีย์ที่พิมพ์ผิดผ่านคอมไพเลอร์ไปโผล่เป็นข้อความดิบบนจอ
  function formatNote(id: string): string {
    if (id === 'pptx') return t('workbench.deckPptxEditable')
    if (id === 'pptx-img') return t('workbench.deckPptxPicture')
    return ''
  }

  async function exportAs(id: string) {
    menuOpen = false
    failure = ''
    landed = ''
    busy = true
    try {
      landed = await ExportDeck(path, id)
    } catch (err) {
      failure = String(err)
    } finally {
      busy = false
    }
  }

  // OpenExport, not OpenFileExternally: an export now lands in the machine's
  // Downloads folder, which is outside the project, and every path-taking
  // binding here refuses that on purpose. The Go side gates this one on the set
  // of files it actually wrote instead.
  const fileNameOf = (p: string) => p.split(/[\\/]/).pop() || p

  async function openLanded() {
    if (!landed) return
    try {
      await OpenExport(landed)
    } catch (err) {
      failure = t('workbench.openFileError', { err: String(err) })
    }
  }
</script>

<svelte:body on:click={() => (menuOpen = false)} />

<svelte:window on:keydown={(e) => !source && onKey(e)} />

<div class="deck-pane">
  <div class="deck-head">
    <span class="deck-name" title={name}>{name}</span>

    <div class="deck-views" role="group">
      <button type="button" class="seg" class:on={!source} onclick={() => (source = false)}>
        <Icon name="layoutList" size={13} /> {t('workbench.deckSlides')}
      </button>
      <button type="button" class="seg" class:on={source} onclick={() => (source = true)}>
        <Icon name="fileCode" size={13} /> {t('workbench.deckSource')}
      </button>
    </div>

    {#if !source}
      <div class="deck-nav">
        <button
          type="button" class="ctrl icon-only" onclick={() => goto(current - 1)}
          disabled={current <= 0} aria-label={t('workbench.deckPrev')} title={t('workbench.deckPrev')}
        ><Icon name="arrowLeft" size={13} /></button>
        <span class="deck-count">{slides.length === 0 ? 0 : current + 1} / {slides.length}</span>
        <button
          type="button" class="ctrl icon-only" onclick={() => goto(current + 1)}
          disabled={current >= slides.length - 1} aria-label={t('workbench.deckNext')} title={t('workbench.deckNext')}
        ><Icon name="arrowRight" size={13} /></button>
      </div>
      <button type="button" class="ctrl" onclick={present}>
        <Icon name="monitor" size={13} /> {t('workbench.deckPresent')}
      </button>

      <!-- stopPropagation เพราะ <svelte:body> ปิดเมนูอยู่ ถ้าไม่กัน คลิกเปิดกับ
           คลิกปิดจะเป็นคลิกเดียวกัน แล้วเมนูจะไม่มีวันเปิดค้าง -->
      <div class="deck-export" onclick={(e) => e.stopPropagation()} role="none">
        <button
          type="button" class="ctrl" disabled={busy}
          aria-haspopup="menu" aria-expanded={menuOpen}
          onclick={() => (menuOpen = !menuOpen)}
        >
          {#if busy}<span class="spin"><Icon name="loaderCircle" size={13} /></span>
          {:else}<Icon name="download" size={13} />{/if}
          {busy ? t('workbench.deckExporting') : t('workbench.deckExport')}
          {#if !busy}<Icon name="chevronDown" size={12} />{/if}
        </button>

        {#if menuOpen}
          <div class="deck-menu" role="menu">
            {#each formats as f (f.id)}
              <!-- แถวที่ยังไม่พร้อมแสดงไว้แต่กดไม่ได้ พร้อมบอกเหตุผลตรงนั้น
                   แถวที่หายไปเฉย ๆ ทำให้คนไม่รู้ว่ามันกำลังจะมา ส่วนแถวที่ดู
                   กดได้แล้วปฏิเสธ คือคำโกหกที่ต้องกดถึงจะรู้ -->
              <button
                type="button" class="deck-menu-item" role="menuitem"
                disabled={!f.ready} onclick={() => exportAs(f.id)}
              >
                <span class="mi-ext">{f.ext}</span>
                <!-- สองแถวมีนามสกุลเดียวกันได้ (.pptx สองแบบ) เมนูที่โชว์แต่
                     นามสกุลจึงอ่านเหมือนรายการซ้ำ คำต่อท้ายคือสิ่งที่บอกว่า
                     ต่างกันตรงไหน และมันอยู่ในไฟล์ภาษา ไม่ใช่ในฝั่ง Go -->
                {#if !f.ready}
                  <span class="mi-note">{t('workbench.deckFormatNotReady')}</span>
                {:else if formatNote(f.id)}
                  <span class="mi-note">{formatNote(f.id)}</span>
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  {#if failure}
    <div class="deck-note err">{failure}</div>
  {:else if landed}
    <!-- แอปนี้ไม่มีระบบ toast โดยตั้งใจ การแจ้งเตือนจึงเป็นแถบที่อยู่กับที่จนกว่า
         จะปิดหรือส่งออกใหม่ ไม่ใช่ของที่วาบแล้วหายไปก่อนคนอ่านทัน

         โชว์ชื่อไฟล์ ไม่ใช่พาธเต็ม รอบก่อนพาธเต็มยาวจนโดนตัดกลางคัน แล้วส่วนที่
         ถูกตัดคือชื่อไฟล์ ซึ่งเป็นส่วนเดียวที่คนอยากรู้ พาธเต็มย้ายไปอยู่ใน title -->
    <div class="deck-done" role="status">
      <span class="dn-mark"><Icon name="check" size={13} /></span>
      <span class="dn-text" title={landed}>
        <strong>{fileNameOf(landed)}</strong>
        <span class="dn-where">{t('workbench.deckSavedToDownloads')}</span>
      </span>
      <button type="button" class="dn-open" onclick={openLanded}>{t('workbench.deckOpenExport')}</button>
      <button
        type="button" class="dn-x" onclick={() => (landed = '')}
        aria-label={t('workbench.deckDismiss')} title={t('workbench.deckDismiss')}
      ><Icon name="x" size={13} /></button>
    </div>
  {/if}

  {#if source}
    <div class="deck-body"><FileEditor {path} {content} /></div>
  {:else}
    <div class="deck-stage" bind:this={stage}>
      <iframe bind:this={frame} {src} title={name} onload={scan}></iframe>
      {#if busy}
        <!-- ระหว่างส่งออก เว็บวิวที่เรนเดอร์อยู่จอดนอกจอในหน้าต่างของตัวเอง
             (deck_render.go) ไม่มีอะไรให้ดู แถบนี้จึงเป็นสิ่งเดียวที่บอกว่ากำลัง
             ทำอยู่ ถ้าไม่มี การกดปุ่มแล้วเงียบไปสองสามวินาทีอ่านเหมือนแอปค้าง -->
        <div class="deck-working" role="status" aria-live="polite">
          <span class="spin lg"><Icon name="loaderCircle" size={22} /></span>
          <span class="dw-title">{t('workbench.deckBuilding')}</span>
          <span class="dw-sub">{t('workbench.deckBuildingSub')}</span>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .deck-pane { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .deck-head {
    display: flex; align-items: center; gap: 8px; padding: 6px 8px;
    border-bottom: 1px solid var(--border-default); flex: none;
  }
  .deck-name {
    font-size: var(--fs-sm); color: var(--text-muted);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .deck-views { display: flex; margin-left: auto; flex: none; }
  .deck-nav { display: flex; align-items: center; gap: 4px; flex: none; }
  .deck-count {
    font-size: var(--fs-sm); color: var(--text-muted);
    font-variant-numeric: tabular-nums; min-width: 44px; text-align: center;
  }

  .deck-head .ctrl, .deck-views .seg {
    flex: none; appearance: none;
    background: var(--surface-sunken); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); color: var(--text-secondary);
    font: inherit; font-size: var(--fs-sm); padding: 4px 10px; cursor: pointer;
    display: inline-flex; align-items: center; gap: 5px;
  }
  .deck-head .ctrl:hover, .deck-views .seg:hover { color: var(--text-primary); }
  .deck-head .ctrl:disabled { opacity: 0.45; cursor: default; }
  .deck-head .icon-only { padding: 4px 7px; }

  /* ปุ่มสองอันติดกันเป็นชิ้นเดียว ขอบตรงกลางจึงมีเส้นเดียวไม่ใช่สองเส้นซ้อน */
  .deck-views .seg:first-child { border-radius: var(--r-sm) 0 0 var(--r-sm); }
  .deck-views .seg:last-child { border-radius: 0 var(--r-sm) var(--r-sm) 0; margin-left: -1px; }
  .deck-views .seg.on { color: var(--text-primary); background: var(--surface-raised); border-color: var(--border-strong); }

  .deck-note { padding: 6px 10px; font-size: var(--fs-sm); flex: none; }
  .deck-note.err { color: var(--text-danger, #f87171); }
  /* แถบสำเร็จ: สามส่วนที่ไม่แย่งที่กัน — เครื่องหมายกับปุ่มกว้างคงที่ ข้อความเป็น
     ตัวเดียวที่ยอมหด และทั้งแถวห้ามขึ้นบรรทัดใหม่ เพราะรอบก่อนปุ่มตกบรรทัดแล้ว
     โดนขอบขวาตัดจนกดไม่ได้ */
  .deck-done {
    flex: none; display: flex; align-items: center; gap: 10px;
    padding: 7px 10px; min-width: 0; flex-wrap: nowrap;
    font-size: var(--fs-sm); color: var(--text-primary);
    background: color-mix(in srgb, var(--good, #16a34a) 14%, transparent);
    border-top: 1px solid color-mix(in srgb, var(--good, #16a34a) 45%, transparent);
  }
  .dn-mark { flex: none; display: inline-flex; color: var(--good, #16a34a); }
  .dn-text {
    min-width: 0; display: flex; align-items: baseline; gap: 8px;
    overflow: hidden; white-space: nowrap;
  }
  .dn-text strong { font-weight: 600; overflow: hidden; text-overflow: ellipsis; }
  .dn-where { color: var(--text-muted); flex: none; }
  .dn-open {
    margin-left: auto; flex: none; white-space: nowrap; appearance: none;
    background: var(--surface-raised); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); color: var(--text-primary);
    font: inherit; font-size: var(--fs-sm); padding: 3px 12px; cursor: pointer;
  }
  .dn-open:hover { border-color: var(--good, #16a34a); }
  .dn-x {
    flex: none; appearance: none; background: none; border: 0; padding: 2px;
    color: var(--text-muted); cursor: pointer; display: inline-flex;
  }
  .dn-x:hover { color: var(--text-primary); }
  .spin { display: inline-flex; animation: tool-spin 1.2s linear infinite; }

  .deck-export { position: relative; flex: none; }
  .deck-menu {
    position: absolute; top: calc(100% + 4px); right: 0; z-index: 20;
    min-width: 152px; padding: 4px;
    background: var(--surface-raised); border: 1px solid var(--border-strong);
    border-radius: var(--r-sm); box-shadow: 0 8px 24px rgb(0 0 0 / 0.35);
    display: flex; flex-direction: column; gap: 2px;
  }
  .deck-menu-item {
    appearance: none; background: none; border: 0; border-radius: var(--r-sm);
    font: inherit; font-size: var(--fs-sm); color: var(--text-secondary);
    padding: 6px 9px; cursor: pointer; text-align: left;
    display: flex; align-items: baseline; gap: 8px;
  }
  .deck-menu-item:hover:not(:disabled) { background: var(--surface-sunken); color: var(--text-primary); }
  .deck-menu-item:disabled { cursor: default; color: var(--text-muted); opacity: 0.6; }
  .mi-ext { font-variant-numeric: tabular-nums; }
  .mi-note { margin-left: auto; font-size: var(--fs-xs, 11px); color: var(--text-muted); }

  .deck-body { flex: 1; min-height: 0; }
  .deck-stage { flex: 1; min-height: 0; background: var(--surface-sunken); position: relative; }
  .deck-working {
    position: absolute; inset: 0; z-index: 5;
    display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px;
    background: color-mix(in srgb, var(--surface-sunken) 88%, transparent);
    backdrop-filter: blur(2px);
  }
  .dw-title { font-size: var(--fs-md, 15px); color: var(--text-primary); }
  .dw-sub { font-size: var(--fs-sm); color: var(--text-muted); }
  .spin.lg { color: var(--interactive, #6ea8fe); margin-bottom: 4px; }
  .deck-stage iframe { width: 100%; height: 100%; border: 0; display: block; }
  /* เต็มจอแล้วพื้นหลังต้องเป็นของเด็ค ไม่ใช่สีธีมของแอปที่โผล่มาเป็นกรอบ */
  .deck-stage:fullscreen { background: #000; }
</style>
