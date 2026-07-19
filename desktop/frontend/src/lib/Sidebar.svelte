<script lang="ts">
  import type { TreeNode, Session } from './types'
  import { toggleNode, selectNode, selectSession } from './stores/cockpit.svelte'

  let { tree, sessions }: { tree: TreeNode[]; sessions: Session[] } = $props()

  // Flat tree + depth → hide rows under a collapsed folder.
  const visible = $derived.by(() => {
    const out: TreeNode[] = []
    let collapseDepth = Infinity
    for (const n of tree) {
      if (n.depth > collapseDepth) continue
      collapseDepth = Infinity
      out.push(n)
      if (n.kind === 'dir' && !n.open) collapseDepth = n.depth
    }
    return out
  })

  function onRowClick(node: TreeNode) {
    if (node.kind === 'dir') toggleNode(node)
    else selectNode(node)
  }
</script>

<aside class="side">
  <div class="scroll">
    <div class="side-head"><span class="eyebrow">Project</span></div>
    <div class="proj">
      {#each visible as node (node.label + node.depth)}
        <button type="button" class="row indent-{node.depth}" class:active={node.active} onclick={() => onRowClick(node)}>
          {#if node.kind === 'dir'}
            <span class="tw">{node.open ? '▾' : '▸'}</span>
          {/if}
          <span class="ic">{node.icon}</span>
          {node.label}
          {#if node.status === 'M'}<span class="st m">M</span>{/if}
          {#if node.status === 'U'}<span class="st u">U</span>{/if}
        </button>
      {/each}
    </div>

    <div class="side-sec">
      <div class="side-head"><span class="eyebrow">Sessions</span></div>
      {#if sessions.length > 0}
        <div class="muted sess-day">Today</div>
      {/if}
      {#each sessions as s}
        <button type="button" class="sess-row" class:active={s.active} onclick={() => selectSession(s)}>
          <span class="t">{s.title}</span>
          <span class="ago">{s.ago}</span>
          <span class="dot green"></span>
        </button>
      {/each}
      <button class="newbtn">＋ New Session</button>
    </div>
  </div>
</aside>
