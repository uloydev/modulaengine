<script>
  let { collections, activeCollection, currentView, onSelectCollection, onSelectView } = $props();
</script>

<aside class="w-60 border-r border-[#4B5360] bg-[#222831] flex flex-col justify-between select-none">
  <div class="p-3">
    <!-- View Switcher -->
    <div class="grid grid-cols-2 gap-1 p-1 bg-[#393E46] rounded border border-[#4B5360] mb-4">
      <button
        onclick={() => onSelectView('data')}
        class="py-1 text-xs font-medium rounded transition-colors {currentView === 'data' ? 'bg-[#00ADB5] text-[#222831]' : 'text-[#CBD5E1] hover:text-[#EEEEEE]'}"
      >
        Data Grid
      </button>
      <button
        onclick={() => onSelectView('schema')}
        class="py-1 text-xs font-medium rounded transition-colors {currentView === 'schema' ? 'bg-[#00ADB5] text-[#222831]' : 'text-[#CBD5E1] hover:text-[#EEEEEE]'}"
      >
        Schema
      </button>
    </div>

    <!-- Collections Header -->
    <div class="flex items-center justify-between px-2 mb-2">
      <span class="text-[11px] font-semibold uppercase tracking-wider text-[#CBD5E1]">Collections</span>
      <span class="text-[10px] font-mono text-[#CBD5E1]">{collections.length}</span>
    </div>

    <!-- Collections List -->
    <nav class="space-y-0.5">
      {#each collections as col}
        <button
          onclick={() => onSelectCollection(col)}
          class="w-full text-left px-2.5 py-1.5 text-xs rounded transition-colors flex items-center justify-between {activeCollection === col.name ? 'bg-[#00ADB5]/15 text-[#00ADB5] font-semibold border-l-2 border-[#00ADB5]' : 'text-[#EEEEEE] hover:bg-[#393E46]/60'}"
        >
          <div class="flex items-center space-x-2 truncate">
            <svg class="w-3.5 h-3.5 text-[#CBD5E1]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span class="truncate">{col.label || col.name}</span>
          </div>
          <span class="font-mono text-[10px] text-[#CBD5E1]">usr_{col.name}</span>
        </button>
      {/each}
    </nav>
  </div>

  <!-- Bottom System Status -->
  <div class="p-3 border-t border-[#4B5360] bg-[#393E46]/40 text-[11px] text-[#CBD5E1]">
    <div class="flex items-center justify-between">
      <span>Queue Engine:</span>
      <span class="text-[#10B981] font-mono">Running</span>
    </div>
    <div class="flex items-center justify-between mt-1">
      <span>Reaper Daemon:</span>
      <span class="text-[#10B981] font-mono">Active (60s)</span>
    </div>
  </div>
</aside>
