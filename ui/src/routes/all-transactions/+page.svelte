<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The rows this page loaded. Assigning to them overrides the loaded value
	// until the next load, which is how an owner edit shows up immediately.
	let rows = $derived(data.transactions.data);
	let savingId = $state<number | null>(null);
	let saveError = $state('');

	let account = $state('');
	let category = $state('');
	let owner = $state('');
	let from = $state('');
	let to = $state('');

	$effect(() => {
		account = data.params.account ?? '';
		category = data.params.category ?? '';
		owner = data.params.owner ?? '';
		from = data.params.from ?? '';
		to = data.params.to ?? '';
	});

	const limit = $derived(data.transactions.limit);
	const offset = $derived(data.transactions.offset);
	const total = $derived(data.transactions.total);

	const categoryTotals = $derived(() => {
		const map = new Map<string | null, number>(
			data.summary.map((s) => [s.category, Number(s.total)])
		);
		const rows = data.categories.map((cat) => ({
			category: cat.name,
			cadence: cat.cadence,
			total: map.get(cat.name) ?? 0
		}));
		const uncategorized = map.get(null);
		if (uncategorized !== undefined)
			rows.push({ category: null as unknown as string, cadence: 'variable', total: uncategorized });
		return rows.sort((a, b) => a.total - b.total);
	});

	const groups = $derived(() => {
		const all = categoryTotals();
		return (['yearly', 'monthly', 'variable'] as const).map((type) => {
			const rows = all.filter((r) => r.cadence === type);
			return {
				type,
				label: type.charAt(0).toUpperCase() + type.slice(1),
				rows,
				maxAbs: Math.max(1, ...rows.map((r) => Math.abs(r.total)))
			};
		});
	});

	function navigate(newOffset = 0) {
		const p = new URLSearchParams();
		if (account) p.set('account', account);
		if (category) p.set('category', category);
		if (owner) p.set('owner', owner);
		if (from) p.set('from', from);
		if (to) p.set('to', to);
		if (newOffset) p.set('offset', String(newOffset));
		goto(`?${p}`, { replaceState: true });
	}

	function patchRow(id: number, fields: Record<string, unknown>) {
		rows = rows.map((tx) => (Number(tx.id) === id ? { ...tx, ...fields } : tx));
	}

	// setOwner moves one transaction between a partner's personal budget and the
	// joint one. The cell updates first and rolls back if the request fails.
	async function setOwner(tx: Record<string, unknown>, value: string) {
		const id = Number(tx.id);
		const next = value === '' ? null : value;
		const previous = { owner: tx.owner, owner_source: tx.owner_source };

		patchRow(id, { owner: next });
		savingId = id;
		saveError = '';

		try {
			const res = await fetch(`/api/transactions/${id}`, {
				method: 'PATCH',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ owner: next })
			});
			if (!res.ok) throw new Error((await res.text()).trim() || `Request failed (${res.status})`);

			const updated = (await res.json()) as Record<string, unknown>;
			patchRow(id, { owner: updated.owner, owner_source: updated.owner_source });

			// The filter and the category totals both follow ownership, so once a
			// partner is selected the change can move this row out of the page.
			if (owner) await invalidateAll();
		} catch (err) {
			patchRow(id, previous);
			const message = err instanceof Error ? err.message.trim() : '';
			saveError = message || 'Could not save the owner.';
		} finally {
			savingId = null;
		}
	}

	function ownerHint(source: unknown) {
		if (source === 'manual') return 'Set by hand — owner rules will leave it alone';
		if (source === 'rule') return 'Assigned by an owner rule';
		return 'Joint — no owner rule claims it';
	}

	function fmt(amount: unknown) {
		const n = Number(amount);
		return (n < 0 ? '−' : '+') + '$' + Math.abs(n).toFixed(2);
	}
</script>

<main>
	<h1>Transactions</h1>

	<div class="cat-groups">
		{#each groups() as group}
			{#if group.rows.length > 0}
				<table class="table bar-chart">
					<caption>{group.label}</caption>
					<tbody>
						{#each group.rows as row}
							<tr>
								<th scope="row">{row.category ?? 'Uncategorized'}</th>
								<td class="bar">
									{#if row.total !== 0}
										<span
											class="bar__fill"
											class:bar__fill--positive={row.total > 0}
											class:bar__fill--negative={row.total < 0}
											style="--value: {(Math.abs(row.total) / group.maxAbs) * 100}"
										>
											{fmt(row.total)}
										</span>
									{:else}
										<span class="num num--muted">{fmt(row.total)}</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		{/each}
	</div>

	<div class="filters">
		<select class="input" bind:value={account} onchange={() => navigate()}>
			<option value="">All accounts</option>
			{#each data.accounts as a}
				<option value={a}>{a}</option>
			{/each}
		</select>

		<select class="input" bind:value={category} onchange={() => navigate()}>
			<option value="">All categories</option>
			{#each data.categories as c}
				<option value={c.name}>{c.name}</option>
			{/each}
		</select>

		<select class="input" bind:value={owner} onchange={() => navigate()}>
			<option value="">Everyone</option>
			{#each data.owners as o}
				<option value={o}>{o}</option>
			{/each}
			<option value="joint">Joint</option>
		</select>

		<input type="date" class="input" bind:value={from} onchange={() => navigate()} />
		<span>to</span>
		<input type="date" class="input" bind:value={to} onchange={() => navigate()} />

		{#if account || category || owner || from || to}
			<button
				class="btn btn--sm"
				onclick={() => {
					account = '';
					category = '';
					owner = '';
					from = '';
					to = '';
					navigate();
				}}>Clear</button
			>
		{/if}
	</div>

	{#if saveError}
		<p class="error">{saveError}</p>
	{/if}

	<div class="table-scroll">
		<table class="table table--zebra">
			<thead>
				<tr>
					<th>Date</th>
					<th>Reference</th>
					<th>Category</th>
					<th>Account</th>
					<th>Owner</th>
					<th class="num">Amount</th>
				</tr>
			</thead>
			<tbody>
				{#each rows as tx}
					<tr>
						<td class="date">{tx.date}</td>
						<td class="ref">{tx.reference}</td>
						<td>{tx.category}</td>
						<td>{tx.account}</td>
						<td>
							<select
								class="input owner"
								title={ownerHint(tx.owner_source)}
								disabled={savingId === Number(tx.id)}
								value={tx.owner ?? ''}
								onchange={(e) => setOwner(tx, e.currentTarget.value)}
							>
								<option value="">Joint</option>
								{#each data.owners as o}
									<option value={o}>{o}</option>
								{/each}
							</select>
						</td>
						<td class="num" class:num--negative={Number(tx.amount) < 0}>{fmt(tx.amount)}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="empty-row">
							<svg class="icon" aria-hidden="true"><use href="#icon-inbox" /></svg>
							No transactions found.
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="pagination">
		<div class="btn-group">
			<button
				class="btn btn--sm"
				disabled={offset === 0}
				onclick={() => navigate(Math.max(0, offset - limit))}
			>
				← Prev
			</button>
			<button
				class="btn btn--sm"
				disabled={offset + limit >= total}
				onclick={() => navigate(offset + limit)}
			>
				Next →
			</button>
		</div>
		<span>{total === 0 ? '0' : offset + 1}–{Math.min(offset + limit, total)} of {total}</span>
	</div>
</main>

<style>
	main {
		margin: var(--space-4) 0;
		padding: 0 var(--space-4);
	}

	h1 {
		font-family: var(--font-display);
		font-size: var(--font-size-4);
		margin: 0 0 var(--space-4);
	}

	.filters {
		display: flex;
		align-items: center;
		gap: var(--space-4);
		margin-bottom: var(--space-4);
		flex-wrap: wrap;
	}

	.filters .input {
		inline-size: auto;
	}

	.filters span {
		color: var(--neutral-strong);
	}

	.cat-groups {
		display: flex;
		gap: var(--space-6);
		flex-wrap: wrap;
		align-items: flex-start;
		margin-bottom: var(--space-6);
	}

	.cat-groups table {
		inline-size: auto;
		min-inline-size: 18rem;
	}

	caption {
		text-transform: uppercase;
		letter-spacing: var(--letter-spacing-wide);
		font-size: var(--font-size-1);
		color: var(--neutral-strong);
	}

	.ref {
		max-width: 380px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.date {
		color: var(--neutral-strong);
	}

	.owner {
		inline-size: auto;
		color: var(--neutral-strong);
	}

	.error {
		color: var(--negative, #b00020);
		margin: 0 0 var(--space-2);
	}

	.empty-row {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		text-align: center;
		color: var(--neutral-strong);
		padding: var(--space-6) 0;
	}

	.pagination {
		display: flex;
		align-items: center;
		gap: var(--space-4);
		margin-top: var(--space-4);
	}
</style>
