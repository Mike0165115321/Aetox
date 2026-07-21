<script lang="ts">
  import { cockpit, toggleNode, visibleTree } from '../stores/cockpit.svelte'
  import { workbench, openFileTab } from '../stores/workbench.svelte'
  import { t } from '../i18n.svelte'

  const rows = $derived(visibleTree(cockpit.tree))
</script>

<div class="insp-scroll">
  <div class="proj">
    {#each rows as node (node.label + node.depth)}
      <button
        type="button" class="row" class:active={workbench.activeId === 'file-' + node.path}
        style="padding-left:{6 + node.depth * 14}px"
        onclick={() => (node.kind === 'dir' ? toggleNode(node) : openFileTab(node.path))}
      >
        {#if node.kind === 'dir'}
          <span class="tw" class:open={node.open}></span>
        {/if}
        <span class="ic">{node.kind === 'dir' ? (node.open ? '📂' : '📁') : '📄'}</span>
        {node.label}
        {#if node.status === 'M'}<span class="st m">M</span>{/if}
        {#if node.status === 'U'}<span class="st u">U</span>{/if}
      </button>
    {/each}
    {#if cockpit.tree.length === 0}<div class="empty">{t('sidebar.noFiles')}</div>{/if}
  </div>
</div>
