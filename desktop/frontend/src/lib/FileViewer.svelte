<script lang="ts">
  let { path, content }: { path: string; content: string } = $props()

  const lines = $derived(content.split('\n'))
</script>

<div class="file-viewer">
  <div class="fname">{path}</div>
  <div class="diff">
    <pre>{#each lines as line, i}<div class="dl ctx"><span class="ln">{i + 1}</span><span class="tx">{line}</span></div>{/each}</pre>
  </div>
</div>
