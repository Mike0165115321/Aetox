<script lang="ts">
  import { cockpit, selectSession, newSession, searchSessions, toggleNode, visibleTree } from './stores/cockpit.svelte'
  import { openFileTab } from './stores/workbench.svelte'

  let query = $state('')
  let searchTimer: ReturnType<typeof setTimeout> | undefined

  const rows = $derived(visibleTree(cockpit.tree))

  let filesOpen = $state(true)
  let chatOpen = $state(true)
  let filesHeight = $state(280)

  function onSearchInput() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => searchSessions(query), 200)
  }

  function startResize(e: PointerEvent) {
    e.preventDefault()
    const startY = e.clientY
    const startH = filesHeight
    function onMove(ev: PointerEvent) {
      filesHeight = Math.min(Math.max(startH + (ev.clientY - startY), 120), window.innerHeight - 220)
    }
    function onUp() {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }
</script>

<aside class="side">
  <div class="side-panel" style={filesOpen ? `flex:0 0 ${filesHeight}px` : 'flex:0 0 auto'}>
    <button type="button" class="side-head" onclick={() => (filesOpen = !filesOpen)}>
      <span class="chev">{filesOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">ไฟล์ที่ AI กำลังทำงาน</span>
    </button>
    {#if filesOpen}
      <div class="scroll">
        <div class="proj">
          {#each rows as node (node.label + node.depth)}
            <button type="button" class="row" style="padding-left:{6 + node.depth * 14}px" title={node.path} class:active={node.active}
              onclick={() => (node.kind === 'dir' ? toggleNode(node) : openFileTab(node.path))}>
              {#if node.kind === 'dir'}
                <span class="tw">{node.open ? '▾' : '▸'}</span>
              {/if}
              <span class="ic">{node.icon ?? '📄'}</span> {node.label}
              {#if node.status === 'M'}<span class="st m">M</span>{/if}
              {#if node.status === 'U'}<span class="st u">U</span>{/if}
            </button>
          {/each}
          {#if cockpit.tree.length === 0}
            <div class="empty">ยังไม่มีไฟล์ — เปิดโฟลเดอร์ก่อน</div>
          {/if}
        </div>
      </div>
    {/if}
  </div>

  {#if filesOpen && chatOpen}
    <div class="side-resize" role="separator" aria-orientation="horizontal" onpointerdown={startResize}></div>
  {/if}

  <div class="side-panel grow">
    <button type="button" class="side-head" onclick={() => (chatOpen = !chatOpen)}>
      <span class="chev">{chatOpen ? '▾' : '▸'}</span>
      <span class="eyebrow">ประวัติแชท{cockpit.project.name ? ` — ${cockpit.project.name}` : ''}</span>
    </button>
    {#if chatOpen}
      <div class="scroll">
        <input class="sess-search" placeholder="ค้นหาประวัติ…" bind:value={query} oninput={onSearchInput} />
        {#each cockpit.sessions as s (s.id)}
          <button type="button" class="sess-row" class:active={s.active} onclick={() => selectSession(s)}>
            <span class="sess-line">
              <span class="t">{s.title}</span>
              <span class="ago">{s.ago}</span>
              {#if s.active}<span class="dot green"></span>{/if}
            </span>
            {#if s.snippet}<span class="snip">{s.snippet}</span>{/if}
          </button>
        {/each}
        {#if cockpit.sessions.length === 0}
          <div class="empty">{query.trim() ? 'ไม่พบผลลัพธ์' : 'ยังไม่มีประวัติในโปรเจกต์นี้'}</div>
        {/if}
        <button class="newbtn" onclick={newSession}>＋ New Session</button>
      </div>
    {/if}
  </div>
</aside>
