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

	const spend = $derived(data.summary.filter((r) => r.total < 0).sort((a, b) => a.total - b.total));

	const maxAbs = $derived(spend.length ? Math.abs(spend[0].total) : 1);

	function fmt(n: number) {
		return '$' + Math.abs(n).toFixed(2);
	}
</script>

<main>
	<h1>By Category</h1>

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

	{#if spend.length === 0}
		<div class="empty">
			<svg class="empty__icon icon" aria-hidden="true"><use href="#icon-tag" /></svg>
			<p class="empty__text">No data.</p>
		</div>
	{:else}
		<table class="table table--zebra bar-chart">
			<thead>
				<tr>
					<th>Category</th>
					<th class="num">Txns</th>
					<th>Total</th>
				</tr>
			</thead>
			<tbody>
				{#each spend as row}
					<tr>
						<th scope="row">{row.category ?? '—'}</th>
						<td class="num">{row.count}</td>
						<td class="bar">
							<span
								class="bar__fill bar__fill--negative"
								style="--value: {(Math.abs(row.total) / maxAbs) * 100}">{fmt(row.total)}</span
							>
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
</style>
