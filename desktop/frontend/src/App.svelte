<script lang="ts">
  import TopBar from './lib/TopBar.svelte'
  import Sidebar from './lib/Sidebar.svelte'
  import Chat from './lib/Chat.svelte'
  import Inspector from './lib/Inspector.svelte'
  import { onMount } from 'svelte'
  import {
    cockpit, sendUserMessage, loadRealState, openFolder,
    switchProvider, switchThinkLevel, switchApprovalMode,
    switchModel, submitAPIKey,
  } from './lib/stores/cockpit.svelte'

  // cockpit starts as emptyCockpitState(); loadRealState() fills project/model in
  // with what the Go engine actually has. tree/sessions/diff/test panels fill in
  // once a real Go-core data source is wired for them too.
  onMount(() => {
    loadRealState()
  })
</script>

<div class="app">
  <TopBar project={cockpit.project} onOpenFolder={openFolder} />
  <Sidebar tree={cockpit.tree} sessions={cockpit.sessions} />
  <main class="main">
    <Chat
      messages={cockpit.chat}
      task={cockpit.task}
      governanceFile={cockpit.project.governanceFile}
      model={cockpit.model}
      onSend={sendUserMessage}
      onSwitchProvider={switchProvider}
      onSwitchThinkLevel={switchThinkLevel}
      onSwitchApprovalMode={switchApprovalMode}
      onSwitchModel={switchModel}
      onSubmitAPIKey={submitAPIKey}
    />
  </main>
  <aside class="inspector">
    <Inspector
      changedFiles={cockpit.changedFiles}
      diff={cockpit.diff}
      test={cockpit.test}
      commandHistory={cockpit.commandHistory}
      task={cockpit.task}
    />
  </aside>
</div>
