<script lang="ts">
  // ห้องสไลด์ — ที่ทางของเด็คโดยเฉพาะ ไม่ใช่ไฟล์ที่บังเอิญเปิดขึ้นมาได้
  //
  // แพเนลไฟล์ตอบคำถาม "ไฟล์นี้คืออะไร" ห้องนี้ตอบอีกคำถามหนึ่งที่แพเนลไฟล์ตอบ
  // ไม่ได้ คือ "งานนำเสนอที่ทำไว้มีอะไรบ้าง" ซึ่งเป็นคำถามที่คนถามตอนยังไม่รู้ว่า
  // จะเปิดไฟล์ไหน และเป็นเหตุผลที่มันเป็นห้อง singleton เหมือนไฟล์กับเครื่องมือ
  // แทนที่จะเป็นแท็บต่อไฟล์
  //
  // ตัวอ่านไม่ได้เขียนซ้ำที่นี่ ห้องนี้ยืม SlidesPane มาทั้งตัว ทั้งการเดินสไลด์
  // การนำเสนอ และแถบส่งออก ถ้าห้องมีตัวอ่านของตัวเอง ก็จะมีสองที่ที่ตอบคำถาม
  // เดียวกันว่าเด็คหน้าตายังไง และวันที่สองที่นั้นไม่ตรงกันคือวันที่ต้องมาไล่ว่า
  // อันไหนถูก
  import { onMount, onDestroy } from 'svelte'
  import { ListDecksIn, ReadFile } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import type { main } from '../../../wailsjs/go/models'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import { dayBucket } from '../dayBucket'
  import SlidesPane from './SlidesPane.svelte'

  let decks = $state<main.Deck[]>([])
  let chosen = $state('')
  let content = $state('')
  let loading = $state(true)
  let failure = $state('')

  // ช่วงเวลาเดียวกับหน้าผลงาน และไม่ใช่เพราะอยากให้หน้าตาเหมือนกัน แถวในผลงาน
  // ราคาเท่ากับ readdir กับ stat แต่แถวที่นี่ต้องอ่านไฟล์ทั้งไฟล์แล้วแจงเป็น
  // HTML รูปฝังอยู่ในเด็คตามสัญญาของมัน เด็คที่มีภาพจึงหนักเป็นเมกะไบต์ ห้องนี้
  // โหลดใหม่ทุกครั้งที่เทิร์นจบด้วย ช่วงเวลาจึงเป็นเพดานที่นี่มากกว่าที่โน่น
  //
  // `range` คือสิ่งที่คนเลือก `served` คือสิ่งที่ฝั่งโกตอบกลับมา สองอันต่างกัน
  // เมื่อช่วงที่เลือกว่างแล้วมันขยายให้เอง ปุ่มจึงผูกกับ served ไม่ใช่ range
  const RANGES = ['week', 'month', 'all'] as const
  type Range = (typeof RANGES)[number]
  let range = $state<Range>('week')
  let served = $state<Range>('week')
  let total = $state(0)

  // ทุกแถวในช่วงมาครบในคำตอบเดียว ตัวนี้คุมแค่ว่าวาดกี่แถว เท่ากับที่ผลงานทำ
  const PAGE = 30
  let shown = $state(PAGE)

  const picked = $derived(decks.find((d) => d.path === chosen))

  // แถวที่วาดจริง พร้อมหัววันของแต่ละแถว dayBucket ใช้ร่วมกับประวัติแชท ฟีดงาน
  // ในออฟฟิศ และหน้าผลงาน หัวจะโผล่เฉพาะตอนที่วันเปลี่ยน รายการจึงอ่านเป็น
  // ไทม์ไลน์ ไม่ใช่กำแพง
  const visible = $derived(decks.slice(0, shown))
  const rows = $derived(
    visible.map((d, i) => ({
      deck: d,
      head:
        i === 0 || dayBucket(d.modified) !== dayBucket(visible[i - 1].modified)
          ? dayBucket(d.modified)
          : null,
    })),
  )

  async function load() {
    try {
      const page = await ListDecksIn(range)
      decks = page.decks ?? []
      served = (RANGES as readonly string[]).includes(page.range) ? (page.range as Range) : 'all'
      total = page.total
      // กลับไปหนึ่งหน้าจอทุกครั้งที่ชุดข้อมูลเปลี่ยนใต้เท้า ไม่งั้นกดไป "ทั้งหมด"
      // แล้วมันวาดหกร้อยแถวทันที ซึ่งคือราคาที่ทั้งเรื่องนี้ตั้งใจเลี่ยง
      shown = PAGE
      failure = ''
    } catch (err) {
      failure = String(err)
    } finally {
      loading = false
    }
    // เด็คที่เพิ่งทำเสร็จคือเด็คที่คนกำลังจะดู รายการเรียงใหม่สุดก่อนอยู่แล้ว
    // จึงเลือกตัวแรกให้เลย ห้องที่เปิดมาแล้วว่างเปล่าทั้งที่มีเด็คอยู่ ทำให้ต้อง
    // คลิกอีกทีเพื่อดูสิ่งที่ตั้งใจมาดูตั้งแต่แรก
    if (!decks.some((d) => d.path === chosen)) await choose(decks[0]?.path ?? '')
  }

  async function choose(path: string) {
    chosen = path
    content = ''
    if (!path) return
    try {
      content = await ReadFile(path)
    } catch (err) {
      failure = String(err)
    }
  }

  function pick(next: Range) {
    range = next
    void load()
  }

  onMount(load)
  // เอเจนต์เขียนเด็คเสร็จระหว่างที่ห้องเปิดค้างอยู่ได้ รายการจึงต้องตามงานที่
  // เพิ่งผลิต ไม่ใช่ค้างอยู่ที่ตอนเปิดห้อง `agent:done` คือจุดที่เทิร์นจบและไฟล์
  // ที่มันเขียนอยู่บนดิสก์ครบแล้ว ส่วน `workspace:changed` เพราะรายการนี้ผูกกับ
  // โปรเจกต์ที่เปิดอยู่ (ListDecks) เปลี่ยนโปรเจกต์แล้วรายการเดิมเป็นของที่อื่น
  const offDone = EventsOn('agent:done', load)
  const offSpace = EventsOn('workspace:changed', load)
  onDestroy(() => {
    offDone()
    offSpace()
  })

  function when(iso: string): string {
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
  }
</script>

<div class="room">
  <aside class="list">
    <div class="list-head">
      <span>{t('deckRoom.title')}</span>
      <button type="button" class="refresh" onclick={load} aria-label={t('deckRoom.refresh')} title={t('deckRoom.refresh')}>
        <Icon name="refreshCw" size={13} />
      </button>
    </div>

    {#if loading}
      <p class="empty">{t('deckRoom.loading')}</p>
    {:else if failure}
      <p class="empty err">{failure}</p>
    {:else if decks.length === 0}
      <!-- ห้องว่างต้องบอกว่าจะทำให้มันไม่ว่างได้ยังไง ไม่ใช่แค่บอกว่าว่าง -->
      <p class="empty">{t('deckRoom.none')}</p>
    {:else}
      <!-- ช่วงที่เห็นอยู่จริง ผูกกับ served ไม่ใช่กับสิ่งที่กด สัปดาห์ที่ว่าง
           ถูกขยายมาแล้วฝั่งโก ปุ่มต้องบอกสิ่งที่อยู่บนจอ -->
      <div class="ranges">
        {#each RANGES as r (r)}
          <button type="button" class="range" class:on={served === r} onclick={() => pick(r)}>
            {t(`artifacts.range.${r}`)}
          </button>
        {/each}
      </div>
      <ul>
        {#each rows as row (row.deck.path)}
          {@const d = row.deck}
          {#if row.head}
            <li class="head">{t(row.head)}</li>
          {/if}
          <li>
            <button type="button" class="row" class:on={d.path === chosen} onclick={() => choose(d.path)}>
              <span class="row-name" title={d.path}>{d.name}</span>
              <span class="row-meta">{t('deckRoom.slideCount', { n: String(d.slides) })} · {when(d.modified)}</span>
            </button>
          </li>
        {/each}
        <!-- ทุกแถวในช่วงมาถึงแล้ว ปุ่มนี้ตัดสินแค่ว่าวาดเพิ่มกี่แถว จำนวนคือสาระ
             "แสดงเพิ่ม" เฉย ๆ ไม่บอกว่าซ่อนอยู่สี่แถวหรือสี่ร้อย -->
        {#if decks.length > shown}
          <li>
            <button type="button" class="more" onclick={() => (shown += PAGE)}>
              <Icon name="chevronDown" size={13} />
              {t('deckRoom.more', { n: String(decks.length - shown) })}
            </button>
          </li>
        {/if}
      </ul>
      <p class="count">{t('deckRoom.count', { n: String(total) })}</p>
    {/if}
  </aside>

  <div class="stage">
    {#if picked && content}
      <!-- คีย์ที่พาธ เพื่อให้เปลี่ยนเด็คแล้วตัวอ่านเริ่มใหม่จริง ไม่ใช่ค้างที่
           สไลด์ที่ ๕ ของเด็คก่อนหน้า -->
      {#key picked.path}
        <SlidesPane path={picked.path} name={picked.name} {content} />
      {/key}
    {:else if !loading && decks.length > 0}
      <p class="empty">{t('deckRoom.pick')}</p>
    {/if}
  </div>
</div>

<style>
  .room { display: flex; height: 100%; min-height: 0; }

  .list {
    width: 248px; flex: none; display: flex; flex-direction: column;
    border-right: 1px solid var(--border-default); min-height: 0;
  }
  .list-head {
    display: flex; align-items: center; gap: 8px; padding: 8px 10px;
    font-size: var(--fs-sm); color: var(--text-muted);
    border-bottom: 1px solid var(--border-default); flex: none;
  }
  .refresh {
    margin-left: auto; appearance: none; background: none; border: 0;
    color: var(--text-muted); cursor: pointer; padding: 2px; display: inline-flex;
  }
  .refresh:hover { color: var(--text-primary); }

  .ranges {
    display: flex; gap: 4px; padding: 6px 8px 0; flex: none;
  }
  .range {
    appearance: none; background: none; border: 1px solid transparent;
    border-radius: var(--r-sm); color: var(--text-muted); cursor: pointer;
    font: inherit; font-size: var(--fs-xs, 11px); padding: 3px 8px;
  }
  .range:hover { color: var(--text-primary); }
  .range.on { background: var(--surface-raised); border-color: var(--border-strong); color: var(--text-primary); }

  .list ul { list-style: none; margin: 0; padding: 6px; overflow-y: auto; min-height: 0; }
  .list li.head {
    padding: 10px 10px 4px; font-size: var(--fs-xs, 11px); color: var(--text-muted);
  }
  .more {
    width: 100%; appearance: none; background: none; border: 0; cursor: pointer;
    color: var(--text-muted); font: inherit; font-size: var(--fs-sm);
    padding: 8px 10px; display: flex; align-items: center; gap: 6px;
  }
  .more:hover { color: var(--text-primary); }
  .count {
    flex: none; margin: 0; padding: 6px 10px; font-size: var(--fs-xs, 11px);
    color: var(--text-muted); border-top: 1px solid var(--border-default);
  }
  .row {
    width: 100%; text-align: left; appearance: none; background: none;
    border: 1px solid transparent; border-radius: var(--r-sm);
    padding: 8px 10px; cursor: pointer; font: inherit;
    display: flex; flex-direction: column; gap: 3px;
  }
  .row:hover { background: var(--surface-sunken); }
  .row.on { background: var(--surface-raised); border-color: var(--border-strong); }
  .row-name {
    font-size: var(--fs-sm); color: var(--text-primary);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .row-meta { font-size: var(--fs-xs, 11px); color: var(--text-muted); }

  .stage { flex: 1; min-width: 0; min-height: 0; }

  .empty { padding: 16px 12px; font-size: var(--fs-sm); color: var(--text-muted); line-height: 1.6; }
  .empty.err { color: var(--text-danger, #f87171); }
</style>
