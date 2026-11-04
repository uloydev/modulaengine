<script>
  let { collection, activeTenant } = $props();

  let loading = $state(false);
  let error = $state(null);
  let search = $state('');
  let sortField = $state('id');
  let sortOrder = $state('ASC');
  let page = $state(0);
  let pageSize = $state(25);

  let editingCell = $state(null); // { rowId, colName }
  let editValue = $state('');

  // Sample data records adhering to dual-identifier pattern
  let records = $state([
    {
      id: '018e32c8-8869-7c91-b3fa-e866184a1d52',
      order_number: 'ORD-2026-001',
      total_amount: 150000.00,
      status: 'PAID',
      version: 1,
      created_at: '2026-09-15T10:00:00Z',
      updated_at: '2026-09-15T10:00:00Z'
    },
    {
      id: '018e32c8-8869-7c91-b3fa-e866184a1d53',
      order_number: 'ORD-2026-002',
      total_amount: 75000.50,
      status: 'DRAFT',
      version: 2,
      created_at: '2026-09-15T11:30:00Z',
      updated_at: '2026-09-15T12:00:00Z'
    },
    {
      id: '018e32c8-8869-7c91-b3fa-e866184a1d54',
      order_number: 'ORD-2026-003',
      total_amount: 320000.00,
      status: 'CANCELLED',
      version: 1,
      created_at: '2026-09-15T14:15:00Z',
      updated_at: '2026-09-15T14:15:00Z'
    }
  ]);

  let columns = $derived.by(() => {
    if (!collection || !collection.fields) return ['id', 'order_number', 'total_amount', 'status', 'version', 'created_at'];
    let cols = ['id'];
    for (let f of collection.fields) {
      cols.push(f.name);
    }
    cols.push('version', 'created_at');
    return cols;
  });

  let filteredRecords = $derived.by(() => {
    let list = records;
    if (search.trim() !== '') {
      let q = search.toLowerCase();
      list = list.filter(r => {
        return Object.values(r).some(v => String(v).toLowerCase().includes(q));
      });
    }
    return list;
  });

  function toggleSort(col) {
    if (sortField === col) {
      sortOrder = sortOrder === 'ASC' ? 'DESC' : 'ASC';
    } else {
      sortField = col;
      sortOrder = 'ASC';
    }
  }

  function startEdit(rowId, colName, value) {
    if (colName === 'id' || colName === 'version' || colName === 'created_at') return;
    editingCell = { rowId, colName };
    editValue = String(value ?? '');
  }

  function saveEdit(rowId, colName) {
    let rec = records.find(r => r.id === rowId);
    if (rec) {
      rec[colName] = editValue;
      rec.version += 1;
      rec.updated_at = new Date().toISOString();
    }
    editingCell = null;
  }

  function cancelEdit() {
    editingCell = null;
  }
</script>

<div class="flex-1 flex flex-col overflow-hidden bg-[#222831]">
  <!-- Grid Control Bar -->
  <div class="h-12 border-b border-[#4B5360] px-4 flex items-center justify-between bg-[#222831]">
    <div class="flex items-center space-x-3">
      <h2 class="text-sm font-semibold text-[#EEEEEE] flex items-center space-x-2">
        <span>{collection.label || collection.name}</span>
        <span class="text-xs font-normal text-[#CBD5E1]">({filteredRecords.length} records)</span>
      </h2>

      <!-- Search Box -->
      <div class="relative ml-4">
        <input
          type="text"
          placeholder="Filter records..."
          bind:value={search}
          class="w-64 pl-8 pr-3 py-1 text-xs rounded bg-[#393E46] text-[#EEEEEE] border border-[#4B5360] placeholder-[#CBD5E1]/60 focus:outline-none focus:ring-1 focus:ring-[#00ADB5]"
        />
        <svg class="w-3.5 h-3.5 text-[#CBD5E1] absolute left-2.5 top-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      </div>
    </div>

    <div class="flex items-center space-x-2">
      <!-- Create Record Button -->
      <button
        onclick={() => {
          let newRec = {
            id: crypto.randomUUID(),
            order_number: `ORD-2026-${String(records.length + 1).padStart(3, '0')}`,
            total_amount: 0.00,
            status: 'DRAFT',
            version: 1,
            created_at: new Date().toISOString()
          };
          records = [newRec, ...records];
        }}
        class="px-3 py-1 text-xs font-semibold rounded bg-[#00ADB5] text-[#222831] hover:bg-[#00c4ce] transition-colors focus:outline-none focus:ring-1 focus:ring-[#00ADB5]"
      >
        + Add Record
      </button>
    </div>
  </div>

  <!-- Data Grid Body -->
  <div class="flex-1 overflow-auto">
    {#if loading}
      <!-- Skeleton Loading State -->
      <div class="p-4 space-y-2">
        {#each Array(6) as _}
          <div class="h-8 bg-[#393E46]/60 rounded animate-pulse"></div>
        {/each}
      </div>
    {:else if error}
      <!-- Error State -->
      <div class="p-8 text-center">
        <div class="inline-flex p-3 rounded-full bg-[#F87171]/20 text-[#F87171] mb-2">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </div>
        <h3 class="text-sm font-semibold text-[#EEEEEE]">Query Execution Failed</h3>
        <p class="text-xs text-[#CBD5E1] mt-1">{error}</p>
        <button
          onclick={() => error = null}
          class="mt-3 px-3 py-1 text-xs rounded bg-[#393E46] text-[#EEEEEE] border border-[#4B5360] hover:bg-[#434A54]"
        >
          Retry
        </button>
      </div>
    {:else if filteredRecords.length === 0}
      <!-- Empty State -->
      <div class="p-12 text-center select-none">
        <div class="w-10 h-10 mx-auto rounded bg-[#393E46] flex items-center justify-center text-[#CBD5E1] mb-3">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
          </svg>
        </div>
        <h3 class="text-sm font-medium text-[#EEEEEE]">No records found</h3>
        <p class="text-xs text-[#CBD5E1] mt-1">Collection '{collection.name}' currently has no entries matching your filter.</p>
      </div>
    {:else}
      <!-- Virtual Data Table -->
      <table class="w-full text-left border-collapse text-xs">
        <thead class="sticky top-0 bg-[#222831] border-b border-[#4B5360] z-10">
          <tr>
            {#each columns as col}
              <th
                onclick={() => toggleSort(col)}
                class="px-3 py-2 text-[11px] font-semibold uppercase tracking-wider text-[#CBD5E1] cursor-pointer hover:text-[#EEEEEE] transition-colors border-r border-[#4B5360]/40"
              >
                <div class="flex items-center space-x-1">
                  <span>{col}</span>
                  {#if sortField === col}
                    <span class="text-[#00ADB5] font-mono">{sortOrder === 'ASC' ? '↑' : '↓'}</span>
                  {/if}
                </div>
              </th>
            {/each}
          </tr>
        </thead>
        <tbody class="divide-y divide-[#4B5360]/40">
          {#each filteredRecords as row}
            <tr class="h-8 hover:bg-[#393E46]/60 transition-colors">
              {#each columns as col}
                <td
                  ondblclick={() => startEdit(row.id, col, row[col])}
                  class="px-3 py-1.5 text-[#EEEEEE] truncate max-w-[200px] border-r border-[#4B5360]/20 {col === 'id' ? 'font-mono text-[11px] text-[#00ADB5]' : ''}"
                >
                  {#if editingCell && editingCell.rowId === row.id && editingCell.colName === col}
                    <!-- Inline Editor -->
                    <input
                      type="text"
                      bind:value={editValue}
                      onkeydown={(e) => {
                        if (e.key === 'Enter') saveEdit(row.id, col);
                        if (e.key === 'Escape') cancelEdit();
                      }}
                      onblur={() => saveEdit(row.id, col)}
                      class="w-full px-1.5 py-0.5 text-xs rounded bg-[#222831] text-[#EEEEEE] border border-[#00ADB5] focus:outline-none"
                    />
                  {:else if col === 'status'}
                    <!-- Status Badge -->
                    <span class="px-1.5 py-0.5 rounded text-[10px] font-semibold tracking-wider uppercase {row[col] === 'PAID' ? 'bg-[#10B981]/20 text-[#10B981]' : row[col] === 'DRAFT' ? 'bg-[#F59E0B]/20 text-[#F59E0B]' : 'bg-[#F87171]/20 text-[#F87171]'}">
                      {row[col]}
                    </span>
                  {:else if col === 'version'}
                    <span class="font-mono text-[11px] text-[#CBD5E1]">v{row[col]}</span>
                  {:else if col === 'total_amount'}
                    <span class="font-mono">${Number(row[col] || 0).toFixed(2)}</span>
                  {:else}
                    <span title={String(row[col] ?? '')}>{String(row[col] ?? '')}</span>
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>

  <!-- Bottom Pagination Bar -->
  <div class="h-9 border-t border-[#4B5360] px-4 flex items-center justify-between text-[11px] text-[#CBD5E1] bg-[#222831]">
    <span>Showing {filteredRecords.length} of {records.length} records</span>
    <div class="flex items-center space-x-2">
      <button
        disabled={page === 0}
        onclick={() => page = Math.max(0, page - 1)}
        class="px-2 py-0.5 rounded bg-[#393E46] text-[#EEEEEE] border border-[#4B5360] disabled:opacity-50"
      >
        Prev
      </button>
      <span class="font-mono">Page {page + 1}</span>
      <button
        onclick={() => page += 1}
        class="px-2 py-0.5 rounded bg-[#393E46] text-[#EEEEEE] border border-[#4B5360]"
      >
        Next
      </button>
    </div>
  </div>
</div>
