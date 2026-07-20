<script lang="ts">
  import { onMount } from 'svelte'
  import Terminal from './Terminal.svelte'
  import { TerminalShells, TerminalStart, TerminalClose } from '../../wailsjs/go/main/App'

  type Tab = { id: string; name: string }

  let tabs = $state<Tab[]>([])
  let activeId = $state('')
  let shells = $state<{ name: string; path: string }[]>([])
  let menuOpen = $state(false)

  onMount(async () => {
    shells = await TerminalShells()
  })

  async function openShell(shell: { name: string; path: string }) {
    menuOpen = false
    const id = await TerminalStart(shell.path, 80, 24)
    tabs.push({ id, name: shell.name })
    activeId = id
  }

  function removeTab(id: string) {
    const idx = tabs.findIndex((t) => t.id === id)
    if (idx === -1) return
    tabs.splice(idx, 1)
    if (activeId === id) activeId = tabs.at(-1)?.id ?? ''
  }

  async function closeTab(id: string) {
    await TerminalClose(id)
    removeTab(id)
  }

  function closeMenuOnOutsideClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.plus-menu-wrap')) menuOpen = false
  }
</script>

<svelte:window onclick={menuOpen ? closeMenuOnOutsideClick : undefined} onkeydown={(e) => e.key === 'Escape' && (menuOpen = false)} />

<div class="term-dock">
  <div class="tabs term-tabs">
    {#each tabs as t (t.id)}
      <button class="tab" class:active={activeId === t.id} onclick={() => (activeId = t.id)}>
        {t.name}
        <span
          class="tab-close" role="button" tabindex="0" aria-label={`Close ${t.name}`}
          onclick={(e) => { e.stopPropagation(); closeTab(t.id) }}
          onkeydown={(e) => e.key === 'Enter' && closeTab(t.id)}
        >✕</span>
      </button>
    {/each}
    <div class="plus-menu-wrap">
      <button class="icobtn tiny plus-btn" aria-label="New terminal" onclick={() => (menuOpen = !menuOpen)}>+</button>
      {#if menuOpen}
        <div class="plus-menu">
          {#each shells as s}
            <button class="plus-menu-item" onclick={() => openShell(s)}>{s.name}</button>
          {/each}
          {#if shells.length === 0}<div class="empty">ไม่พบ shell ในเครื่อง</div>{/if}
        </div>
      {/if}
    </div>
  </div>
  <div class="term-body">
    {#each tabs as t (t.id)}
      <div class="term-slot" style="display:{activeId === t.id ? 'block' : 'none'}">
        <Terminal sessionId={t.id} onExit={() => removeTab(t.id)} />
      </div>
    {/each}
    {#if tabs.length === 0}
      <div class="empty term-empty">กด + เพื่อเปิดเทอร์มินัล</div>
    {/if}
  </div>
</div>
