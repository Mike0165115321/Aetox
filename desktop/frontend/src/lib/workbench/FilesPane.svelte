<script lang="ts">
  import { cockpit, toggleNode, visibleTree } from '../stores/cockpit.svelte'
  import { openFileTab } from '../stores/workbench.svelte'

  const rows = $derived(visibleTree(cockpit.tree))
</script>

<div class="insp-scroll">
  <div class="proj">
    {#each rows as node (node.label + node.depth)}
      <button type="button" class="row" style="padding-left:{6 + node.depth * 14}px" class:active={node.active} onclick={() => (node.kind === 'dir' ? toggleNode(node) : openFileTab(node.path))}>
        {#if node.kind === 'dir'}
          <span class="tw">{node.open ? '▾' : '▸'}</span>
        {/if}
        <span class="ic">{node.icon}</span>
        {node.label}
        {#if node.status === 'M'}<span class="st m">M</span>{/if}
        {#if node.status === 'U'}<span class="st u">U</span>{/if}
      </button>
    {/each}
    {#if cockpit.tree.length === 0}<div class="empty">ยังไม่มีไฟล์ — เปิดโฟลเดอร์ก่อน</div>{/if}
  </div>
</div>
