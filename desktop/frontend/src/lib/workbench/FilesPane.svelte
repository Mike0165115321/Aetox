<script lang="ts">
  import { cockpit, openFolder, toggleNode, visibleTree } from '../stores/cockpit.svelte'
  import { workbench, openFileTab, setTabDragPayload } from '../stores/workbench.svelte'
  import { t } from '../i18n.svelte'
  import Icon from '../Icon.svelte'

  const rows = $derived(visibleTree(cockpit.tree))
</script>

<div class="insp-scroll">
  <div class="proj">
    {#each rows as node (node.label + node.depth)}
      <!-- Draggable, because this is where a file actually lives: the tab strip
           and the reply's file cards could both be dragged to the composer or
           the desk, and the tree — the one place you go to *find* a file —
           could not. A folder is not: there is nothing to attach. -->
      <button
        type="button" class="row" class:active={workbench.activeId === 'file-' + node.path}
        style="padding-left:{6 + node.depth * 14}px"
        onclick={() => (node.kind === 'dir' ? toggleNode(node) : openFileTab(node.path))}
        draggable={node.kind !== 'dir'}
        ondragstart={(e) => setTabDragPayload(e, 'file', node.path, node.label)}
      >
        {#if node.kind === 'dir'}
          <span class="tw" class:open={node.open}></span>
        {/if}
        <span class="ic"><Icon name={node.kind === 'dir' ? (node.open ? 'folderOpen' : 'folder') : 'fileText'} size={13} /></span>
        {node.label}
        {#if node.status === 'M'}<span class="st m">M</span>{/if}
        {#if node.status === 'U'}<span class="st u">U</span>{/if}
      </button>
    {/each}
    {#if cockpit.tree.length === 0}
      <div class="pane-empty">
        <p>{t('sidebar.noFiles')}</p>
        <button type="button" class="proj-add" onclick={openFolder}>
          <span class="ic"><Icon name="folderOpen" size={14} /></span> {t('topbar.openFolder')}
        </button>
      </div>
    {/if}
  </div>
</div>
