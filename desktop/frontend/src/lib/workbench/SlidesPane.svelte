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
  import { onDestroy, onMount } from 'svelte'
  import { DeckFormats, ExportDeck, OpenExport, OpenFileExternally } from '../../../wailsjs/go/main/App'
  import type { main } from '../../../wailsjs/go/models'
  import FileEditor from '../FileEditor.svelte'
  import { DECK_BASE, deckFit, documentScrolls, sendStepKey, slideElements, typedIntoField, visibleIndex } from './deckNav'
  import { deckPick, startDeckPick, stopDeckPick, type PickMode } from './pagePick.svelte'
  import { fileURL } from '../fileUrl'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  // `active` is the tab being the one on screen. The desk keeps every tab
  // mounted and hides the others (Workbench.svelte's display:none slots), so a
  // deck opened an hour ago is still listening while the user reads a browser
  // tab — and would step, unseen, on every arrow key pressed anywhere in the
  // app. Defaults to true: a room rendered on its own is the room being looked
  // at, which is what the tests do and what any future single-pane caller means.
  let { path, name, content, active = true }: {
    path: string; name: string; content: string; active?: boolean
  } = $props()

  const src = $derived(fileURL(path))

  let frame = $state<HTMLIFrameElement | null>(null)
  let stage = $state<HTMLDivElement | null>(null)
  let slides = $state<HTMLElement[]>([])
  let current = $state(0)
  let source = $state(false)
  let failure = $state('')

  // เด็คมีสองทรง และทั้งสองทรงเป็นของจริง
  //
  // ทรงแรก สไลด์เรียงกันลงมาเป็นสาย แพเนลเลื่อนไปหาทีละใบ — ทรงเดียวที่แพเนลนี้
  // รองรับตอนเขียนครั้งแรก
  //
  // ทรงที่สอง สไลด์ซ้อนกันอยู่ที่เดียว โชว์ทีละใบ แล้วเด็คสลับเอง เป็นทรงที่
  // reveal.js ใช้ และเป็นทรงที่โมเดลเขียนออกมาเองแทบทุกครั้งเมื่อถูกสั่งว่าให้ทำ
  // เด็ค HTML เด็คแบบนี้ไม่มีอะไรให้เลื่อน scrollIntoView จึงไม่ขยับอะไรเลย ปุ่ม
  // ลูกศรของแพเนลตายสนิททุกครั้ง — อาการที่เจ้าของรายงานเมื่อ 2026-08-20
  //
  // แพเนลไม่เดา API ของเด็ค มันกดปุ่มที่เด็คฟังอยู่แล้วทีละก้าว แล้วอ่านกลับว่าใบ
  // ไหนโผล่ขึ้นมา เท่ากับคนกดคีย์บอร์ดหนึ่งครั้ง ไม่มีอะไรถูกฉีดลงไฟล์ของผู้ใช้
  // ซึ่งเป็นกติกาเดียวกับที่หัวไฟล์นี้ประกาศไว้
  let stacked = $state(false)
  // กันเหตุการณ์ที่แพเนลสังเคราะห์เองวนกลับเข้ามาเรียกตัวเองไม่รู้จบ
  let sending = false

  // ห้องเดินสไลด์ ก็เป็นคนบอกเด็คด้วยว่าใบไหนกำลังอยู่บนเวที
  //
  // ติดคลาส .onstage ให้ใบที่เห็นอยู่ ถอดออกจากใบอื่น เด็คทรงซ้อนที่ไม่มีอะไรให้
  // scroll แขวนเอนทรานซ์ไว้กับคลาสนี้ ทุกใบจึงได้อนิเมชั่นของตัวเองตอนถูกเรียก
  // ขึ้นมา ไม่ใช่วิ่งพร้อมกันหมดตอนโหลดแล้วเห็นแค่ใบแรก ส่วนเด็คทรงสายใช้
  // animation-timeline ของ CSS ทำงานเดียวกันจากการเลื่อน จึงไม่ต้องแตะคลาสนี้ก็ได้
  //
  // นี่คือ DOM ที่รันไทม์แตะ ไม่ใช่สิ่งที่เขียนลงไฟล์ (เหมือนการซ่อนแถบเลื่อนใน
  // scan) เด็คบนดิสก์จึงยังเป็น HTML สะอาดที่เปิดที่ไหนก็ได้ ไม่ติดหนี้คลาสนี้
  $effect(() => {
    const list = slides
    const at = current
    for (let i = 0; i < list.length; i++) list[i].classList.toggle('onstage', i === at)
  })

  // อ่านรายการสไลด์ใหม่ทุกครั้งที่ไอเฟรมโหลดเสร็จ ไม่ใช่ครั้งเดียวตอน mount
  // เพราะเอเจนต์เขียนทับไฟล์เดิมบ่อยมาก และ Workbench คีย์แพเนลไว้ที่ rev อยู่
  // แล้ว การอ่านซ้ำตรงนี้กันกรณีที่ไอเฟรมเองนำทางใหม่โดยที่แท็บไม่ถูกอ่านซ้ำ
  function scan() {
    failure = ''
    // A reload is a new document, and the overlay lived in the old one. Without
    // this the toolbar button stays lit over a deck that is not listening — the
    // same reason the browser side unwires on `browser:meta`.
    stopDeckPick(null)
    const doc = frameDoc()
    if (!doc) return
    slides = slideElements(doc)
    stacked = slides.length > 1 && !documentScrolls(doc)
    current = stacked ? readVisible() : 0
    // เด็คทรงซ้อนฟังปุ่มของมันเองอยู่แล้ว ถ้าแพเนลฟังซ้ำในเอกสารเดียวกัน การกด
    // ลูกศรหนึ่งครั้งจะเดินสองใบ หน้าที่ของแพเนลตรงนี้จึงเหลือแค่ตามให้ทัน
    if (stacked) {
      doc.addEventListener('keydown', syncAfterDeckInput, true)
      doc.addEventListener('click', syncAfterDeckInput, true)
    } else {
      doc.addEventListener('keydown', onKey)
    }
    doc.addEventListener('scroll', syncFromScroll, { passive: true, capture: true })
    // แถบเลื่อนกินความกว้างไปจากกรอบ ~15px เด็คที่วัดจากวิวพอร์ตจะจัดหน้าที่ 1265
    // แล้วสิ่งที่ย่อลงมาก็ไม่ใช่ 16:9 อีกต่อไป ห้องนี้เดินสไลด์ให้อยู่แล้ว แถบเลื่อน
    // จึงไม่มีงานทำ — ซ่อนที่เอกสารที่โหลดอยู่ ไม่ใช่ที่ไฟล์
    doc.documentElement.style.setProperty('scrollbar-width', 'none')
    fit()
    // อีกครั้งหลังเฟรมถัดไป เพราะฟอนต์กับรูปอาจยังจัดหน้าไม่นิ่งตอน load ยิง
    // ซึ่งเป็นเหตุผลที่อาการ "บางทีก็เพี้ยน" เพี้ยนไม่เท่ากันทุกครั้ง
    requestAnimationFrame(fit)
  }

  // ย่อเด็คให้พอดีแพเนล
  //
  // สัญญาบอกว่าสไลด์หนึ่งใบคือหน้าขนาด 1280 × 720 แล้วบอกต่อว่า "แพเนลย่อให้พอดี
  // จอจากมัน" ซึ่งแปลว่ากรอบที่เด็ควาดลงไปต้องเป็น 1280 × 720 เสมอ ไม่ว่าแพเนลจะ
  // กว้างแคบแค่ไหน
  //
  // รอบก่อนทำกลับด้าน: ไอเฟรมกินพื้นที่แพเนลทั้งหมด แล้วค่อย `zoom` เอกสารข้างใน
  // ให้เล็กลง วิธีนั้นใช้ได้กับเด็คที่ประกาศขนาดของตัวเองเท่านั้น เด็คที่วัดจาก
  // วิวพอร์ต (`min-height:100vh`, `clamp(...,10vw,...)` — ทรงที่โมเดลเขียนออกมา
  // แทบทุกครั้ง) จะขยายตัวเองเท่ากับแพเนลพอดี อัตราส่วนที่คำนวณได้จึงเป็น 1 เป๊ะ
  // ทุกครั้ง ไม่มีอะไรถูกย่อเลย แล้วเด็คก็ไปจัดหน้าใหม่ตามรูปทรงของแพเนลแทน —
  // ตัวอักษรบวมตามความกว้าง ความสูงถูกบีบ ส่วนที่ล้นโดน `overflow:hidden` ตัดทิ้ง
  // คืออาการ "โดนบีบจนเลื่อนเอง" ที่เจ้าของรายงานเมื่อ 2026-08-20
  //
  // ตอนนี้กรอบมาก่อน: ไอเฟรมถูกตรึงไว้ที่ขนาดสไลด์ แล้วย่อทั้งกรอบด้วย
  // `transform` เด็คที่วัดจากวิวพอร์ตจึงได้วิวพอร์ต 16:9 ที่มันต้องการ ส่วนเด็คที่
  // ประกาศขนาดเองก็ได้ขนาดของมันเหมือนเดิม และการวัดข้างในไอเฟรมเป็นพิกัดก่อนย่อ
  // ทั้งหมด goto กับ syncFromScroll จึงไม่ต้องรู้จักอัตราส่วนเลย
  //
  // ไม่มีอะไรถูกเขียนลงไฟล์ — ขนาดกับ transform อยู่บนแท็ก <iframe> ของแอป เด็ค
  // บนดิสก์ยังเปิดในเบราว์เซอร์ไหนก็ได้เหมือนเดิม
  function fit() {
    const doc = frameDoc()
    const box = stage?.getBoundingClientRect()
    if (!doc || !box || !frame || slides.length === 0 || box.width === 0) return

    // กลับไปที่กรอบมาตรฐานก่อนวัดเสมอ ไม่ใช่วัดจากกรอบที่รอบก่อนตั้งค้างไว้
    frame.style.width = `${DECK_BASE.width}px`
    frame.style.height = `${DECK_BASE.height}px`

    // วัดใบที่เห็นอยู่ ไม่ใช่ใบแรกเสมอ เด็คทรงซ้อนที่ซ่อนด้วย `display:none`
    // ให้กล่องขนาดศูนย์กับใบที่ไม่ได้โชว์ ส่วนกฎว่าอ่านค่านั้นเป็นกรอบยังไง อยู่ที่
    // deckFit
    const { width, height, scale } = deckFit(box, (slides[current] ?? slides[0]).getBoundingClientRect())
    if (width !== DECK_BASE.width || height !== DECK_BASE.height) {
      frame.style.width = `${width}px`
      frame.style.height = `${height}px`
    }

    // พอดีทั้งใบ ไม่ใช่พอดีแค่ด้านกว้าง สไลด์หนึ่งใบต้องอยู่ในสายตาทั้งใบ ไม่งั้น
    // คนดูต้องเลื่อนเพื่ออ่านบรรทัดสุดท้ายของสไลด์ที่ควรเห็นทีเดียวจบ
    frame.style.transform = `translate(-50%, -50%) scale(${scale})`
    // กรอบเปลี่ยนขนาดแล้วตำแหน่งเลื่อนเดิมชี้ไปคนละที่ จึงต้องพากลับมาที่ใบเดิม
    center(slides[current])
  }

  // พาสไลด์มาอยู่กลางจอ โดยเลื่อน "เอกสารในไอเฟรม" เท่านั้น
  //
  // เคยเป็น el.scrollIntoView() ซึ่งเป็นต้นเหตุที่เจ้าของรายงานเมื่อ 2026-08-20
  // ว่า "กดสไลด์ขึ้นมาแล้ว พอโหลดสไลด์แล้วมันดัน" แถบซ้ายกับขวาของแอปถูกดันออก
  // นอกจอ
  //
  // scrollIntoView ไม่ได้เลื่อนแค่กล่องที่ใกล้ที่สุด สเปกบอกให้มันเดินขึ้นไป
  // เลื่อน "ทุกกล่องที่เลื่อนได้" เหนือ element นั้นจนถึงวิวพอร์ต และเอกสารใน
  // ไอเฟรมนี้เป็น same-origin กับหน้าต่างแอป (fileUrl.ts) มันจึงไม่หยุดที่ขอบ
  // ไอเฟรม แต่ไปเลื่อน `.app` ต่อ ซึ่งกว้างกว่าหน้าต่างได้จริงเมื่อผู้ใช้ย่อ
  // หน้าต่างหลังลากแพเนลไว้กว้าง แล้วผลคือทั้งกริดถูกเลื่อนไปข้างหนึ่ง โดยไม่มี
  // สกรอลบาร์ให้เลื่อนกลับเพราะ `.app` ซ่อน overflow ไว้
  //
  // การเซ็ต scrollTop ของเอกสารในไอเฟรมเองไม่มีทางไปแตะบรรพบุรุษข้างนอกได้เลย
  // ซึ่งเป็นคุณสมบัติที่ต้องการ ไม่ใช่แค่ผลข้างเคียง
  function center(el: HTMLElement | undefined, smooth = false) {
    const doc = frameDoc()
    const view = doc?.scrollingElement ?? doc?.documentElement
    if (!el || !view) return
    const box = el.getBoundingClientRect()
    const top = view.scrollTop + box.top - Math.max(0, (view.clientHeight - box.height) / 2)
    if (smooth && typeof view.scrollTo === 'function') view.scrollTo({ top, behavior: 'smooth' })
    else view.scrollTop = top
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

  function readVisible(): number {
    const view = frameDoc()?.defaultView
    return view ? visibleIndex(slides, view, current) : current
  }

  // เดินเด็คทรงซ้อนทีละก้าวด้วยปุ่มที่มันฟังอยู่แล้ว
  //
  // ก้าวละหนึ่งครั้งเพราะเด็คส่วนใหญ่รับได้แค่ "ถัดไป/ก่อนหน้า" — ไม่มีทางบอกมันว่า
  // "ไปใบที่เจ็ด" โดยไม่รู้จัก API ของมัน guard กันไม่ให้กดวนไม่รู้จบถ้าเด็คไม่ขยับ
  function step(target: number) {
    const doc = frameDoc()
    if (!doc) return
    let guard = slides.length + 1
    sending = true
    try {
      while (current !== target && guard-- > 0) {
        sendStepKey(doc, target > current)
        const after = readVisible()
        // เด็คที่ไม่ฟังปุ่มลูกศรจะไม่ขยับ หยุดตรงนี้ ดีกว่ากดต่อจนครบรอบ
        if (after === current) break
        current = after
      }
    } finally {
      sending = false
    }
  }

  // เด็คทรงซ้อนมีปุ่มของตัวเองบนหน้าจอ ผู้ใช้กดปุ่มนั้นได้ ตัวนับของแพเนลจึงต้อง
  // ตามไปด้วย ไม่ใช่ค้างอยู่ที่ 1/12 ขณะที่เด็คเดินไปถึงใบที่สิบ อ่านในเฟรมถัดไป
  // เพราะตัวจัดการของเด็คเองยังไม่ได้ทำงานตอนที่ capture listener นี้ยิง
  function syncAfterDeckInput() {
    if (!stacked) return
    requestAnimationFrame(() => {
      current = readVisible()
    })
  }

  function goto(i: number) {
    if (slides.length === 0) return
    const target = Math.max(0, Math.min(slides.length - 1, i))
    if (stacked) {
      step(target)
      return
    }
    current = target
    center(slides[current], true)
  }

  // ผู้ใช้เลื่อนเองได้ ตัวนับจึงต้องตามการเลื่อนจริง ไม่ใช่ตามปุ่มที่กดล่าสุด
  // เลือกสไลด์ที่ขอบบนอยู่ใกล้ขอบบนจอที่สุด แทนที่จะหารด้วยความสูง เพราะระยะ
  // ห่างระหว่างสไลด์เป็นของ CSS ในไฟล์ ซึ่งแพเนลไม่ควรรู้
  function syncFromScroll() {
    // เด็คทรงซ้อนไม่มีการเลื่อน และทุกใบวางทับกันอยู่ที่เดียว การหาใบที่ใกล้กลาง
    // จอที่สุดจึงตอบ "ใบแรก" เสมอ แล้วลากตัวนับกลับไปที่ 1 ทุกครั้งที่มีอะไรเลื่อน
    if (stacked || slides.length === 0) return
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
    if (sending) return // ปุ่มที่แพเนลเพิ่งกดเข้าไปในเด็คเอง
    // ห้องนี้ฟังทั้งหน้าต่าง คนที่กำลังพิมพ์อยู่จึงต้องได้ปุ่มของตัวเองคืนไปครบ
    // ทุกปุ่ม ไม่ใช่แค่ Space (กฎอยู่ที่ deckNav — มันเป็นคำถามว่าปุ่มเป็นของใคร)
    if (typedIntoField(e.target)) return
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

  // The slide number travels with the pick because a deck is one file: the model
  // is about to be told to change something, and "the h1" is a different element
  // on every one of eight pages.
  function arm(mode: PickMode) {
    const doc = frameDoc()
    if (doc) void startDeckPick(doc, path, current + 1, mode)
  }
  // Leaving the panel with the mode still armed would leave the crosshair on a
  // document nobody can reach any more, and the callback hanging off the window.
  onDestroy(() => stopDeckPick(frameDoc()))

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

<svelte:window on:keydown={(e) => !source && active && onKey(e)} />

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
      <!-- ชี้ให้เอเจนดู, on the slide.
           Beside นำเสนอ rather than in the settings of anything, because it is the
           same act as the browser toolbar's: you are looking at the thing, and the
           button is where you are looking. What it produces is not the same,
           though — a page pick names a URL, a slide pick names the file on disk,
           so the answer to "ทำไมหัวข้อมันใหญ่จัง" can be an edit rather than a
           description. -->
      <button
        type="button" class="ctrl icon-only" class:on={deckPick.path === path && deckPick.mode === 'pick'}
        aria-label={t('workbench.pick')} title={t('workbench.pick')}
        onclick={() => arm('pick')}
      ><Icon name="pointer" size={13} /></button>
      <!-- Drawing ends differently here. A browser tab is photographed by the
           engine; a deck cannot be, so the picture is rendered and the ink laid
           over it (deck_draw.go), which takes a moment — hence the busy overlay
           below answering to `deckPick.capturing` as well as to an export. -->
      <button
        type="button" class="ctrl icon-only" class:on={deckPick.path === path && deckPick.mode === 'draw'}
        aria-label={t('workbench.draw')} title={t('workbench.draw')}
        onclick={() => arm('draw')}
      ><Icon name="pencil" size={13} /></button>
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
          {#if busy || deckPick.capturing}<span class="spin"><Icon name="loaderCircle" size={13} /></span>
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
          <span class="dw-title">{busy ? t('workbench.deckBuilding') : t('workbench.deckShooting')}</span>
          <span class="dw-sub">{busy ? t('workbench.deckBuildingSub') : t('workbench.deckShootingSub')}</span>
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
    /* A flex item's floor is its min-content width, and nowrap text has no width
       smaller than all of it — so without this a long deck name refuses to give
       ground and pushes the controls along instead of ellipsing. */
    flex: 0 1 auto; min-width: 0;
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
  /* ปุ่มที่ติดอยู่ใช้หน้าตาเดียวกับปุ่มสไลด์/ซอร์สที่ถูกเลือก ไม่ใช่สีใหม่ */
  .deck-head .ctrl.on { color: var(--text-primary); background: var(--surface-raised); }

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
  .deck-stage { flex: 1; min-height: 0; background: var(--surface-sunken); position: relative; overflow: hidden; }
  .deck-working {
    position: absolute; inset: 0; z-index: 5;
    display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px;
    background: color-mix(in srgb, var(--surface-sunken) 88%, transparent);
    backdrop-filter: blur(2px);
  }
  .dw-title { font-size: var(--fs-md, 15px); color: var(--text-primary); }
  .dw-sub { font-size: var(--fs-sm); color: var(--text-muted); }
  .spin.lg { color: var(--interactive, #6ea8fe); margin-bottom: 4px; }
  /* กรอบมีขนาดของสไลด์ ไม่ใช่ขนาดของแพเนล แล้ว fit() ย่อทั้งกรอบลงมาวางกลางเวที
     ดูเหตุผลเต็มที่ fit() — ตัวเลขที่นี่เป็นแค่ค่าตั้งต้นก่อนวัดรอบแรก */
  .deck-stage iframe {
    position: absolute; left: 50%; top: 50%;
    width: 1280px; height: 720px; border: 0; display: block;
    transform: translate(-50%, -50%); transform-origin: center center;
  }
  /* เต็มจอแล้วพื้นหลังต้องเป็นของเด็ค ไม่ใช่สีธีมของแอปที่โผล่มาเป็นกรอบ */
  .deck-stage:fullscreen { background: #000; }
</style>
