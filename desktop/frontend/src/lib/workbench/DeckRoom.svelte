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
  import { ListDecks, ReadFile } from '../../../wailsjs/go/main/App'
  import { EventsOn } from '../../../wailsjs/runtime/runtime'
  import type { main } from '../../../wailsjs/go/models'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'
  import SlidesPane from './SlidesPane.svelte'

  let decks = $state<main.Deck[]>([])
  let chosen = $state('')
  let content = $state('')
  let loading = $state(true)
  let failure = $state('')

  const picked = $derived(decks.find((d) => d.path === chosen))

  async function load() {
    try {
      decks = await ListDecks()
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
      <ul>
        {#each decks as d (d.path)}
          <li>
            <button type="button" class="row" class:on={d.path === chosen} onclick={() => choose(d.path)}>
              <span class="row-name" title={d.path}>{d.name}</span>
              <span class="row-meta">{t('deckRoom.slideCount', { n: String(d.slides) })} · {when(d.modified)}</span>
            </button>
          </li>
        {/each}
      </ul>
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

  .list ul { list-style: none; margin: 0; padding: 6px; overflow-y: auto; min-height: 0; }
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
