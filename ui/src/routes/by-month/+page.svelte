<script lang="ts">
	import { goto } from '$app/navigation';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let month = $state('');
	let account = $state('');
	let category = $state('');

	$effect(() => {
		month = data.month;
		account = data.account;
		category = data.category;
	});

	function navigate() {
		const p = new URLSearchParams();
		if (month) p.set('m', month);
		if (account) p.set('account', account);
		if (category) p.set('category', category);
		goto(`?${p}`, { replaceState: true });
	}

	const total = $derived(data.transactions.reduce((sum, tx) => sum + Number(tx.amount), 0));

	const categoryTotals = $derived(() => {
		const map = new Map<string | null, number>(
			data.summary.map((s) => [s.category, Number(s.total)])
		);
		const yearlyMap = new Map<string | null, number>(
			data.yearlySummary.map((s) => [s.category, Number(s.total)])
		);
		const rows = data.categories.map((cat) => ({
			category: cat.name,
			cadence: cat.cadence,
			total: (cat.cadence === 'yearly' ? yearlyMap : map).get(cat.name) ?? 0
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

	function fmt(amount: unknown) {
		const n = Number(amount);
		return (n < 0 ? '−' : '+') + '$' + Math.abs(n).toFixed(2);
	}
</script>

<svelte:head>
	<title>By Month · Finances</title>
</svelte:head>

<main>
	<h1>By Month</h1>

	<div class="filters">
		<input type="month" class="input" bind:value={month} onchange={navigate} />
	</div>

	{#if data.month && data.categories.length > 0}
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
	{/if}

	{#if data.month}
		<div class="filters">
			<select class="input" bind:value={account} onchange={navigate}>
				<option value="">All accounts</option>
				{#each data.accounts as a}
					<option value={a}>{a}</option>
				{/each}
			</select>

			<select class="input" bind:value={category} onchange={navigate}>
				<option value="">All categories</option>
				{#each data.categories as c}
					<option value={c.name}>{c.name}</option>
				{/each}
			</select>

			{#if account || category}
				<button
					class="btn btn--sm"
					onclick={() => {
						account = '';
						category = '';
						navigate();
					}}>Clear</button
				>
			{/if}
		</div>
	{/if}

	{#if !data.month}
		<div class="empty">
			<svg class="empty__icon icon" aria-hidden="true"><use href="#icon-calendar" /></svg>
			<p class="empty__text">Select a month.</p>
		</div>
	{:else if data.transactions.length === 0}
		<div class="empty">
			<svg class="empty__icon icon" aria-hidden="true"><use href="#icon-inbox" /></svg>
			<p class="empty__text">No transactions for {data.month}.</p>
		</div>
	{:else}
		<div class="table-scroll">
			<table class="table table--zebra">
				<thead>
					<tr>
						<th>Date</th>
						<th>Reference</th>
						<th>Category</th>
						<th>Account</th>
						<th class="num">Amount</th>
					</tr>
				</thead>
				<tbody>
					{#each data.transactions as tx}
						<tr>
							<td class="date">{tx.date}</td>
							<td class="ref">{tx.reference}</td>
							<td>{tx.category}</td>
							<td>{tx.account}</td>
							<td class="num" class:num--negative={Number(tx.amount) < 0}>{fmt(tx.amount)}</td>
						</tr>
					{/each}
				</tbody>
				<tfoot>
					<tr>
						<td colspan="4">Total</td>
						<td class="num total" class:num--negative={total < 0}>{fmt(total)}</td>
					</tr>
				</tfoot>
			</table>
		</div>
	{/if}
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
		flex-wrap: wrap;
		margin-bottom: var(--space-4);
	}

	.filters .input {
		inline-size: auto;
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

	.total {
		font-weight: 600;
	}
</style>
