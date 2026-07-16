<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import Inspector from './lib/Inspector.svelte'
  import { cockpit, hydrate, sendUserMessage } from './lib/stores/cockpit.svelte'
  import { MockSource } from './lib/services/cockpit'

  // The one place a data source is chosen. Swap MockSource → WailsSource to feed
  // the Go core; nothing below changes.
  hydrate(new MockSource())
</script>

<div class="app">
  <TopBar project={cockpit.project} model={cockpit.model} />
  <Sidebar tree={cockpit.tree} sessions={cockpit.sessions} />
  <main class="main">
    <Chat
      messages={cockpit.chat}
      task={cockpit.task}
      governanceFile={cockpit.project.governanceFile}
      onSend={sendUserMessage}
    />
  </main>
  <aside class="inspector">
    <Inspector
      changedFiles={cockpit.changedFiles}
      diff={cockpit.diff}
      test={cockpit.test}
      commandHistory={cockpit.commandHistory}
    />
  </aside>
</div>
