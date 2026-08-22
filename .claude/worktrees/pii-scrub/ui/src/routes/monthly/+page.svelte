<script lang="ts">
	import { goto } from '$app/navigation';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let from = $state('');
	let to = $state('');

	$effect(() => {
		from = data.params.from ?? '';
		to = data.params.to ?? '';
	});

	function navigate() {
		const p = new URLSearchParams();
		if (from) p.set('from', from);
		if (to) p.set('to', to);
		goto(`?${p}`, { replaceState: true });
	}

	const rows = $derived([...data.monthly].reverse());

	const maxSpend = $derived(rows.length ? Math.max(...rows.map((r) => Math.abs(r.spend))) : 1);

	function fmt(n: number) {
		return '$' + Math.abs(n).toFixed(2);
	}
</script>

<main>
	<h1>Monthly</h1>

	<div class="filters">
		<input type="date" class="input" bind:value={from} onchange={navigate} />
		<span>to</span>
		<input type="date" class="input" bind:value={to} onchange={navigate} />
		{#if from || to}
			<button
				class="btn btn--sm"
				onclick={() => {
					from = '';
					to = '';
					navigate();
				}}>Clear</button
			>
		{/if}
	</div>

	{#if rows.length === 0}
		<div class="empty">
			<svg class="empty__icon icon" aria-hidden="true"><use href="#icon-chart-line" /></svg>
			<p class="empty__text">No data.</p>
		</div>
	{:else}
		<table class="table table--zebra bar-chart">
			<thead>
				<tr>
					<th>Month</th>
					<th class="num">Spend</th>
					<th class="num">Income</th>
					<th class="num">Net</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each rows as row}
					<tr>
						<th scope="row" class="month">{row.month}</th>
						<td class="num num--negative">{fmt(row.spend)}</td>
						<td class="num num--positive">{fmt(row.income)}</td>
						<td class="num" class:num--negative={row.net < 0} class:num--positive={row.net > 0}>
							{row.net < 0 ? '−' : '+'}{fmt(row.net)}
						</td>
						<td class="bar">
							<span
								class="bar__fill bar__fill--negative"
								style="--value: {(Math.abs(row.spend) / maxSpend) * 100}"
							></span>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
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
		margin-bottom: var(--space-4);
	}

	.filters .input {
		inline-size: auto;
	}

	.filters span {
		color: var(--neutral-strong);
	}

	.month {
		font-variant-numeric: tabular-nums;
	}
</style>
