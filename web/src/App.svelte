<script>
  import Navbar from './components/Navbar.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import DataGrid from './components/DataGrid.svelte';
  import SchemaDesigner from './components/SchemaDesigner.svelte';

  let isDark = $state(true);
  let currentView = $state('data'); // 'data' | 'schema'

  let activeTenant = $state({
    id: 1,
    name: 'Default Tenant',
    slug: 'default'
  });

  let collections = $state([
    {
      name: 'orders',
      label: 'Customer Orders',
      fields: [
        { name: 'order_number', type: 'text', required: true, searchable: true },
        { name: 'total_amount', type: 'number', precision: 14, scale: 2 },
        { name: 'status', type: 'enum', options: ['DRAFT', 'PAID', 'CANCELLED'] }
      ]
    },
    {
      name: 'products',
      label: 'Products & Catalog',
      fields: [
        { name: 'title', type: 'text', required: true, searchable: true },
        { name: 'slug', type: 'text', required: true, unique: true },
        { name: 'price', type: 'number', precision: 14, scale: 2 },
        { name: 'is_published', type: 'boolean', default: 'false' }
      ]
    },
    {
      name: 'customers',
      label: 'Customers',
      fields: [
        { name: 'email', type: 'text', required: true, unique: true, searchable: true },
        { name: 'first_name', type: 'text', required: true },
        { name: 'last_name', type: 'text' }
      ]
    },
    {
      name: 'deals',
      label: 'Sales Pipeline',
      fields: [
        { name: 'title', type: 'text', required: true, searchable: true },
        { name: 'amount', type: 'number', precision: 14, scale: 2 },
        { name: 'stage', type: 'enum', options: ['PROSPECT', 'NEGOTIATION', 'CLOSED_WON'] }
      ]
    }
  ]);

  let activeCollectionName = $state('orders');

  let activeCollection = $derived.by(() => {
    return collections.find(c => c.name === activeCollectionName) || collections[0];
  });

  function toggleTheme() {
    isDark = !isDark;
    if (isDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }

  function handleSelectCollection(col) {
    activeCollectionName = col.name;
  }

  function handleSelectView(view) {
    currentView = view;
  }

  function handleTenantChange(t) {
    activeTenant = t;
  }
</script>

<div class="h-screen w-screen flex flex-col overflow-hidden bg-[#222831]">
  <!-- Top Global Navigation Bar -->
  <Navbar
    {activeTenant}
    {isDark}
    onTenantChange={handleTenantChange}
    onToggleTheme={toggleTheme}
  />

  <div class="flex-1 flex overflow-hidden">
    <!-- Left Navigation Sidebar -->
    <Sidebar
      {collections}
      activeCollection={activeCollectionName}
      {currentView}
      onSelectCollection={handleSelectCollection}
      onSelectView={handleSelectView}
    />

    <!-- Main Workspace View -->
    <main class="flex-1 flex overflow-hidden">
      {#if currentView === 'data'}
        <DataGrid
          collection={activeCollection}
          {activeTenant}
        />
      {:else}
        <SchemaDesigner
          collection={activeCollection}
        />
      {/if}
    </main>
  </div>
</div>
