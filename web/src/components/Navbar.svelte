<script>
  let { activeTenant, onTenantChange, onToggleTheme, isDark } = $props();

  let tenants = [
    { id: 1, name: 'Default Tenant', slug: 'default' },
    { id: 2, name: 'Acme Enterprise', slug: 'acme' },
    { id: 3, name: 'Staging Sandbox', slug: 'sandbox' }
  ];

  let isDropdownOpen = $state(false);

  function selectTenant(t) {
    onTenantChange(t);
    isDropdownOpen = false;
  }
</script>

<header class="h-14 border-b border-[#4B5360] bg-[#222831] px-4 flex items-center justify-between select-none">
  <div class="flex items-center space-x-4">
    <div class="flex items-center space-x-2">
      <div class="w-7 h-7 rounded bg-[#00ADB5] flex items-center justify-center font-bold text-[#222831] text-sm">
        M
      </div>
      <span class="font-bold text-sm tracking-wide text-[#EEEEEE]">ModulaEngine</span>
      <span class="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-[#393E46] text-[#CBD5E1] border border-[#4B5360]">
        v1.0.0
      </span>
    </div>

    <!-- Tenant Switcher Dropdown -->
    <div class="relative ml-4">
      <button
        onclick={() => isDropdownOpen = !isDropdownOpen}
        class="flex items-center space-x-2 px-2.5 py-1 text-xs rounded bg-[#393E46] text-[#EEEEEE] border border-[#4B5360] hover:bg-[#434A54] transition-colors focus:outline-none focus:ring-1 focus:ring-[#00ADB5]"
      >
        <span class="text-[#CBD5E1]">Tenant:</span>
        <span class="font-medium">{activeTenant.name}</span>
        <svg class="w-3.5 h-3.5 text-[#CBD5E1]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {#if isDropdownOpen}
        <div class="absolute left-0 mt-1 w-48 rounded bg-[#393E46] border border-[#4B5360] shadow-lg py-1 z-50">
          {#each tenants as t}
            <button
              onclick={() => selectTenant(t)}
              class="w-full text-left px-3 py-1.5 text-xs text-[#EEEEEE] hover:bg-[#00ADB5]/20 hover:text-[#00ADB5] transition-colors flex items-center justify-between"
            >
              <span>{t.name}</span>
              <span class="font-mono text-[10px] text-[#CBD5E1]">#{t.id}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>

  <div class="flex items-center space-x-3">
    <!-- Live SSE Pulse -->
    <div class="flex items-center space-x-1.5 px-2 py-0.5 rounded bg-[#393E46] border border-[#4B5360] text-[11px] text-[#CBD5E1]">
      <span class="w-2 h-2 rounded-full bg-[#10B981] animate-pulse"></span>
      <span class="font-mono">Realtime Live</span>
    </div>

    <!-- Theme Toggle -->
    <button
      onclick={onToggleTheme}
      class="p-1.5 rounded bg-[#393E46] text-[#CBD5E1] border border-[#4B5360] hover:text-[#EEEEEE] hover:bg-[#434A54] transition-colors"
      title="Toggle Light/Dark Theme"
      aria-label="Toggle Theme"
    >
      {#if isDark}
        <!-- Sun icon -->
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
        </svg>
      {:else}
        <!-- Moon icon -->
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
        </svg>
      {/if}
    </button>
  </div>
</header>
