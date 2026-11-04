<script>
  let { collection } = $props();

  let newFieldName = $state('');
  let newFieldType = $state('text');
  let isRequired = $state(false);
  let isUnique = $state(false);
  let isSearchable = $state(false);
  let defaultValue = $state('');

  let fieldTypes = ['text', 'number', 'boolean', 'datetime', 'enum', 'json', 'relation'];

  function handleAddField() {
    if (!newFieldName.trim()) return;

    let field = {
      name: newFieldName.trim().toLowerCase(),
      type: newFieldType,
      required: isRequired,
      unique: isUnique,
      searchable: isSearchable,
      default: defaultValue ? defaultValue : undefined,
    };

    if (!collection.fields) collection.fields = [];
    collection.fields.push(field);

    // Reset form
    newFieldName = '';
    newFieldType = 'text';
    isRequired = false;
    isUnique = false;
    isSearchable = false;
    defaultValue = '';
  }
</script>

<div class="flex-1 overflow-auto p-6 bg-[#222831]">
  <div class="max-w-4xl mx-auto space-y-6">
    <!-- Header -->
    <div class="border-b border-[#4B5360] pb-4 flex items-center justify-between">
      <div>
        <h2 class="text-base font-semibold text-[#EEEEEE] flex items-center space-x-2">
          <span>Collection Schema: {collection.label || collection.name}</span>
          <span class="text-xs font-mono text-[#00ADB5] bg-[#00ADB5]/10 px-2 py-0.5 rounded">usr_{collection.name}</span>
        </h2>
        <p class="text-xs text-[#CBD5E1] mt-1">Physical PostgreSQL table with dual-identifiers and composite tenant constraints.</p>
      </div>

      <span class="text-xs font-mono text-[#CBD5E1]">Lock Timeout: 2.0s</span>
    </div>

    <!-- Fields Table -->
    <div class="bg-[#393E46] rounded border border-[#4B5360] overflow-hidden">
      <div class="px-4 py-2.5 bg-[#222831]/60 border-b border-[#4B5360] font-semibold text-xs text-[#CBD5E1] uppercase tracking-wider">
        Defined Fields ({collection.fields?.length || 0})
      </div>
      <table class="w-full text-left text-xs border-collapse">
        <thead>
          <tr class="border-b border-[#4B5360] text-[11px] text-[#CBD5E1]">
            <th class="px-4 py-2">Field Name</th>
            <th class="px-4 py-2">Logical Type</th>
            <th class="px-4 py-2">Physical PostgreSQL Type</th>
            <th class="px-4 py-2">Constraints</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#4B5360]/40">
          <!-- Standard system fields -->
          <tr class="h-8 text-[#CBD5E1] bg-[#222831]/30">
            <td class="px-4 py-1.5 font-mono text-[11px] text-[#00ADB5]">id</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">internal</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">BIGINT GENERATED ALWAYS AS IDENTITY</td>
            <td class="px-4 py-1.5"><span class="px-1.5 py-0.5 rounded bg-[#393E46] text-[10px] font-mono">PRIMARY KEY</span></td>
          </tr>
          <tr class="h-8 text-[#CBD5E1] bg-[#222831]/30">
            <td class="px-4 py-1.5 font-mono text-[11px] text-[#00ADB5]">public_id</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">uuid</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">UUID DEFAULT gen_random_uuid()</td>
            <td class="px-4 py-1.5"><span class="px-1.5 py-0.5 rounded bg-[#393E46] text-[10px] font-mono">UNIQUE(tenant, id)</span></td>
          </tr>
          <tr class="h-8 text-[#CBD5E1] bg-[#222831]/30">
            <td class="px-4 py-1.5 font-mono text-[11px] text-[#00ADB5]">tenant_id</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">foreign_key</td>
            <td class="px-4 py-1.5 font-mono text-[11px]">BIGINT REFERENCES sys_tenants(id)</td>
            <td class="px-4 py-1.5"><span class="px-1.5 py-0.5 rounded bg-[#393E46] text-[10px] font-mono">CASCADE</span></td>
          </tr>

          <!-- User fields -->
          {#each collection.fields || [] as field}
            <tr class="h-8 text-[#EEEEEE] hover:bg-[#434A54]/50">
              <td class="px-4 py-1.5 font-mono font-medium">{field.name}</td>
              <td class="px-4 py-1.5">
                <span class="px-1.5 py-0.5 rounded bg-[#222831] text-[10px] font-mono uppercase text-[#00ADB5]">
                  {field.type}
                </span>
              </td>
              <td class="px-4 py-1.5 font-mono text-[11px] text-[#CBD5E1]">
                {field.type === 'text' ? 'TEXT' : field.type === 'number' ? 'NUMERIC(14,2)' : field.type === 'boolean' ? 'BOOLEAN' : field.type === 'json' ? 'JSONB' : field.type === 'datetime' ? 'TIMESTAMPTZ' : 'VARCHAR(64)'}
              </td>
              <td class="px-4 py-1.5 flex items-center space-x-1.5">
                {#if field.required}
                  <span class="px-1 py-0.5 rounded bg-[#F59E0B]/20 text-[#F59E0B] text-[10px]">REQUIRED</span>
                {/if}
                {#if field.unique}
                  <span class="px-1 py-0.5 rounded bg-[#00ADB5]/20 text-[#00ADB5] text-[10px]">UNIQUE</span>
                {/if}
                {#if field.searchable}
                  <span class="px-1 py-0.5 rounded bg-[#10B981]/20 text-[#10B981] text-[10px]">INDEXED</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Add Field Panel -->
    <div class="bg-[#393E46] rounded border border-[#4B5360] p-4">
      <h3 class="text-xs font-semibold uppercase tracking-wider text-[#CBD5E1] mb-3">Add Custom Field</h3>
      <form onsubmit={(e) => { e.preventDefault(); handleAddField(); }} class="space-y-4">
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label for="new-field-name" class="block text-[11px] font-medium text-[#CBD5E1] mb-1">Field Name</label>
            <input
              id="new-field-name"
              type="text"
              placeholder="e.g. sku_code"
              bind:value={newFieldName}
              class="w-full px-2.5 py-1.5 text-xs rounded bg-[#222831] text-[#EEEEEE] border border-[#4B5360] focus:ring-1 focus:ring-[#00ADB5] focus:outline-none font-mono"
            />
          </div>

          <div>
            <label for="new-field-type" class="block text-[11px] font-medium text-[#CBD5E1] mb-1">Field Type</label>
            <select
              id="new-field-type"
              bind:value={newFieldType}
              class="w-full px-2.5 py-1.5 text-xs rounded bg-[#222831] text-[#EEEEEE] border border-[#4B5360] focus:ring-1 focus:ring-[#00ADB5] focus:outline-none"
            >
              {#each fieldTypes as t}
                <option value={t}>{t}</option>
              {/each}
            </select>
          </div>

          <div>
            <label for="new-field-default" class="block text-[11px] font-medium text-[#CBD5E1] mb-1">Default Value</label>
            <input
              id="new-field-default"
              type="text"
              placeholder="e.g. 0 or now()"
              bind:value={defaultValue}
              class="w-full px-2.5 py-1.5 text-xs rounded bg-[#222831] text-[#EEEEEE] border border-[#4B5360] focus:ring-1 focus:ring-[#00ADB5] focus:outline-none"
            />
          </div>
        </div>

        <div class="flex items-center space-x-6 text-xs text-[#CBD5E1]">
          <label class="flex items-center space-x-2 cursor-pointer">
            <input type="checkbox" bind:checked={isRequired} class="rounded border-[#4B5360] text-[#00ADB5] focus:ring-0 bg-[#222831]" />
            <span>Required (NOT NULL)</span>
          </label>
          <label class="flex items-center space-x-2 cursor-pointer">
            <input type="checkbox" bind:checked={isUnique} class="rounded border-[#4B5360] text-[#00ADB5] focus:ring-0 bg-[#222831]" />
            <span>Unique Index</span>
          </label>
          <label class="flex items-center space-x-2 cursor-pointer">
            <input type="checkbox" bind:checked={isSearchable} class="rounded border-[#4B5360] text-[#00ADB5] focus:ring-0 bg-[#222831]" />
            <span>Searchable (B-Tree Index)</span>
          </label>
        </div>

        <div class="flex justify-end pt-2">
          <button
            type="submit"
            class="px-4 py-1.5 text-xs font-semibold rounded bg-[#00ADB5] text-[#222831] hover:bg-[#00c4ce] transition-colors focus:ring-1 focus:ring-[#00ADB5] focus:outline-none"
          >
            Apply Field Migration
          </button>
        </div>
      </form>
    </div>
  </div>
</div>
