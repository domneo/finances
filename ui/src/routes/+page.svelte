<script lang="ts">
	import { goto } from '$app/navigation';
	import type { PageData } from './$types';
	import type { Recurring } from './+page.ts';

	let { data }: { data: PageData } = $props();

	let month = $state('');

	$effect(() => {
		month = data.month;
	});

	function navigate() {
		goto(month ? `?m=${month}` : '?', { replaceState: true });
	}

	/** Step the picker a whole month either way; Date rolls the year over for us. */
	function shift(by: number) {
		const [y, mo] = (month || data.month).split('-').map(Number);
		const d = new Date(y, mo - 1 + by);
		month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
		navigate();
	}

	const money = new Intl.NumberFormat('en-SG', {
		style: 'currency',
		currency: 'SGD',
		minimumFractionDigits: 2
	});

	/** Expenses are stored negative; every figure here is a magnitude. */
	function fmt(n: number) {
		return money.format(Math.abs(n));
	}

	/**
	 * No category declares which way it runs, so the figures themselves say it:
	 * anything positive is money coming in. Within a spend category that is a
	 * credit — a refund, an insurance payout — netting off what was spent, and
	 * the magnitude alone would read as a bill of the same size, so those get
	 * marked. Across categories it is what sorts money in from money out.
	 */
	function isCredit(n: number) {
		return n > 0;
	}

	function monthLabel(m: string) {
		if (!m) return '';
		const [y, mo] = m.split('-');
		return new Date(Number(y), Number(mo) - 1).toLocaleString('en-SG', {
			month: 'long',
			year: 'numeric'
		});
	}

	const received = $derived(data.budget.reduce((sum, c) => sum + c.received, 0));

	/**
	 * Bills are the categories that typically take money out — judged on the
	 * trailing average, so a bill that was only refunded this month stays a bill.
	 * A variable category is judged on the month it is reporting, since that is
	 * the only figure it carries.
	 */
	const bills = $derived(data.recurring.filter((r) => !isCredit(r.monthly)));
	const monthlyBills = $derived(bills.filter((r) => r.cadence === 'monthly'));
	const yearlyBills = $derived(bills.filter((r) => r.cadence === 'yearly'));
	const variableSpend = $derived(data.variable.filter((v) => !isCredit(v.total)));

	/** Everything that ran the other way, listed on its own rather than netted
	 *  off the spend: earnings, interest, transfers landing in an account. */
	const moneyIn = $derived(
		[
			...data.recurring
				.filter((r) => isCredit(r.monthly))
				.map((r) => ({ category: r.category, typical: r.monthly, actual: r.actual })),
			...data.variable
				.filter((v) => isCredit(v.total))
				.map((v) => ({ category: v.category, typical: null, actual: v.total }))
		].sort((a, b) => b.actual - a.actual)
	);

	/** Recurring cost carried by this month: yearly bills already spread over 12. */
	const recurringTotal = $derived(bills.reduce((sum, r) => sum + r.monthly, 0));
	const variableTotal = $derived(variableSpend.reduce((sum, v) => sum + v.total, 0));
	const moneyInTotal = $derived(moneyIn.reduce((sum, r) => sum + r.actual, 0));

	const expenses = $derived(recurringTotal + variableTotal);
	const remaining = $derived(received + expenses);
	const overBudget = $derived(remaining < 0);

	type Status = { tone: string; label: string };

	function billStatus(r: Recurring): Status {
		if (r.count > 0) return { tone: 'badge--positive', label: 'Charged' };
		// A yearly bill is not due every month — judge it on the last 12 months.
		if (r.cadence === 'yearly') {
			if (!r.lastPaid) return { tone: 'badge--negative', label: 'Never' };
			const cutoff = new Date(`${data.month}-01`);
			cutoff.setFullYear(cutoff.getFullYear() - 1);
			return r.lastPaid >= cutoff.toISOString().slice(0, 10)
				? { tone: '', label: 'Within year' }
				: { tone: 'badge--warning', label: 'Overdue' };
		}
		return { tone: 'badge--warning', label: 'Not charged' };
	}
</script>

<!-- credit will display with "+" to avoid mistaking as expense -->
{#snippet amount(n: number)}
	{#if isCredit(n)}<span class="credit">+{fmt(n)}</span>{:else}{fmt(n)}{/if}
{/snippet}

<svelte:head>
	<title>Dashboard · Finances</title>
</svelte:head>

<main>
	<header class="head">
		<h1 class="t-display-5">Dashboard</h1>
		<div class="month-selector">
			<button
				type="button"
				class="btn btn--icon btn--md"
				onclick={() => shift(-1)}
				aria-label="Previous month"
			>
				<svg class="icon" aria-hidden="true"><use href="#icon-chevron-left" /></svg>
			</button>
			<input type="month" class="input" bind:value={month} onchange={navigate} />
			<button
				type="button"
				class="btn btn--icon btn--md"
				onclick={() => shift(1)}
				aria-label="Next month"
			>
				<svg class="icon" aria-hidden="true"><use href="#icon-chevron-right" /></svg>
			</button>
		</div>
	</header>

	{#if !data.month}
		<div class="empty">
			<svg class="empty__icon icon" aria-hidden="true"><use href="#icon-inbox" /></svg>
			<p class="empty__text">No transactions yet.</p>
		</div>
	{:else}
		<div class="stats">
			<div class="stats__grid">
				<article
					class="stats__remaining card card--pad"
					class:stats__remaining--negative={overBudget}
				>
					<h3 class="stat__title t-body-2">
						Remaining
						<button
							type="button"
							class="stats__hint"
							popovertarget="remaining-formula"
							aria-label="How this is calculated"
							aria-describedby="remaining-formula"
							style="anchor-name: --remaining-formula"
						>
							<svg class="icon" aria-hidden="true"><use href="#icon-info" /></svg>
						</button>
						<span
							id="remaining-formula"
							popover="hint"
							class="tooltip"
							style="position-anchor: --remaining-formula; position-area: inline-end"
						>
							Budget − Expenses
						</span>
					</h3>
					<p class="stat__value t-body-5">
						{overBudget ? '−' : ''}{fmt(remaining)}
					</p>
					<p class="stats__status badge" class:badge--solid={overBudget}>
						{#if overBudget}
							<span class="stats__status-mark" aria-hidden="true">!</span>
						{:else}
							<span class="badge__dot"></span>
						{/if}
						{overBudget ? 'Over budget' : 'Within budget'}
					</p>
				</article>

				<article class="stats__expenses card card--pad">
					<h3 class="stat__title t-body-2">Expenses</h3>
					<p class="stat__value t-body-4">{@render amount(expenses)}</p>
					<dl class="stats__split dl-inline t-body-2">
						<dt>Recurring</dt>
						<dd>{@render amount(recurringTotal)}</dd>
						<dt>Variable</dt>
						<dd>{@render amount(variableTotal)}</dd>
					</dl>
				</article>

				<article class="stats__budget card card--pad">
					<h3 class="stat__title t-body-2">Budget</h3>
					<p class="stat__value t-body-4">{fmt(received)}</p>
					<dl class="stats__split dl-inline t-body-2">
						{#each data.budget as c, i}
							<dt>{c.name}</dt>
							<dd>
								{#if c.received !== c.expected}
									<button
										type="button"
										class="stats__hint stats__hint--negative"
										popovertarget="expected-{i}"
										aria-label="Expected amount"
										aria-describedby="expected-{i}"
										style="anchor-name: --expected-{i}"
									>
										<svg class="icon" aria-hidden="true"><use href="#icon-circle-warning" /></svg>
									</button>
									<span
										id="expected-{i}"
										popover="hint"
										class="stats__tooltip tooltip"
										style="position-anchor: --expected-{i}; position-area: inline-start"
									>
										Expected {fmt(c.expected)}
									</span>
								{/if}
								{fmt(c.received)}
							</dd>
						{/each}
					</dl>
				</article>
			</div>
		</div>

		{#if moneyIn.length > 0}
			<section>
				<h2 class="t-display-3">Money in</h2>
				<article class="card">
					<div class="table-scroll">
						<table class="table table--rows">
							<thead>
								<tr>
									<th class="category">Category</th>
									<th class="num amount">Typical</th>
									<th class="num amount">This month</th>
								</tr>
							</thead>
							<tbody>
								{#each moneyIn as r}
									<tr>
										<th scope="row">{r.category || 'Uncategorized'}</th>
										<td class="num num--muted">{r.typical === null ? '—' : fmt(r.typical)}</td>
										<td class="num" class:num--muted={r.actual === 0}>
											{@render amount(r.actual)}
										</td>
									</tr>
								{/each}
							</tbody>
							<tfoot>
								<tr class="solid">
									<td colspan="2">Total</td>
									<td class="num">{@render amount(moneyInTotal)}</td>
								</tr>
							</tfoot>
						</table>
					</div>
				</article>
			</section>
		{/if}

		<section>
			<h2 class="t-display-3">Recurring — monthly</h2>
			<article class="card">
				<div class="table-scroll">
					<table class="table table--rows">
						<thead>
							<tr>
								<th class="category">Category</th>
								<th>Status</th>
								<th>Last charge</th>
								<th class="num amount">Typical</th>
								<th class="num amount">This month</th>
							</tr>
						</thead>
						<tbody>
							{#each monthlyBills as r}
								{@const status = billStatus(r)}
								<tr>
									<th scope="row">{r.category}</th>
									<td><span class="badge {status.tone}">{status.label}</span></td>
									<td class="date">{r.lastPaid || '—'}</td>
									<td class="num num--muted">{fmt(r.monthly)}</td>
									<td class="num" class:num--muted={r.count === 0}>
										{#if r.count > 0}{@render amount(r.actual)}{:else}—{/if}
										{#if r.count > 1}<span class="hint">×{r.count}</span>{/if}
									</td>
								</tr>
							{/each}
						</tbody>
						<tfoot>
							<tr class="solid">
								<td>Total</td>
								<td colspan="2"></td>
								<td class="num">{fmt(monthlyBills.reduce((s, r) => s + r.monthly, 0))}</td>
								<td class="num">{@render amount(monthlyBills.reduce((s, r) => s + r.actual, 0))}</td
								>
							</tr>
						</tfoot>
					</table>
				</div>
			</article>
		</section>

		<section>
			<h2 class="t-display-3">Recurring — yearly</h2>
			<article class="card">
				<div class="table-scroll">
					<table class="table table--rows">
						<thead>
							<tr>
								<th class="category">Category</th>
								<th>Status</th>
								<th>Last charge</th>
								<th class="num amount">Per month</th>
								<th class="num amount">This month</th>
							</tr>
						</thead>
						<tbody>
							{#each yearlyBills as r}
								{@const status = billStatus(r)}
								<tr>
									<th scope="row">{r.category}</th>
									<td><span class="badge {status.tone}">{status.label}</span></td>
									<td class="date">{r.lastPaid || '—'}</td>
									<td class="num num--muted">{fmt(r.monthly)}</td>
									<td class="num" class:num--muted={r.count === 0}>
										{#if r.count > 0}{@render amount(r.actual)}{:else}—{/if}
									</td>
								</tr>
							{/each}
						</tbody>
						<tfoot>
							<tr class="solid">
								<td>Total</td>
								<td colspan="2"></td>
								<td class="num">{fmt(yearlyBills.reduce((s, r) => s + r.monthly, 0))}</td>
								<td class="num">{@render amount(yearlyBills.reduce((s, r) => s + r.actual, 0))}</td>
							</tr>
						</tfoot>
					</table>
				</div>
			</article>
		</section>

		<section>
			<h2 class="t-display-3">Variable</h2>
			<article class="card">
				<div class="table-scroll">
					<table class="table table--rows">
						<thead>
							<tr>
								<th class="category">Category</th>
								<th class="num">Total</th>
							</tr>
						</thead>
						<tbody>
							{#each variableSpend as v}
								<tr>
									<th scope="row">{v.category || 'Uncategorized'}</th>
									<td class="num" class:num--muted={v.total === 0}>{@render amount(v.total)}</td>
								</tr>
							{/each}
						</tbody>
						<tfoot>
							<tr class="solid">
								<td>Total</td>
								<td class="num">{@render amount(variableTotal)}</td>
							</tr>
						</tfoot>
					</table>
				</div>
			</article>
		</section>
	{/if}
</main>

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-4);
		flex-wrap: wrap;
	}

	.head .input {
		inline-size: auto;
	}

	.month-selector {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	/* the cards size themselves off this box, not the viewport */
	.stats {
		container-type: inline-size;
		margin-block-start: var(--space-8);
	}

	/* one column, then budget/remaining over a full-width expenses, then one row */
	.stats__grid {
		display: grid;
		grid-template-columns: 1fr;
		gap: var(--space-4);
		--card-pad: var(--space-4);
	}

	@container (min-width: 32rem) {
		.stats__grid {
			grid-template-columns: repeat(2, 1fr);
			grid-template-areas:
				'remaining remaining'
				'expenses  budget';
			--card-pad: var(--space-5);
		}

		.stats__budget {
			grid-area: budget;
		}
		.stats__expenses {
			grid-area: expenses;
		}
		.stats__remaining {
			grid-area: remaining;
		}
	}

	@container (min-width: 60rem) {
		.stats__grid {
			grid-template-columns: repeat(3, 1fr);
			grid-template-areas: 'remaining expenses budget';
		}
	}

	/* solid fill — green when there is budget left, red once it is overspent */
	.stats__remaining {
		--card-bg: var(--positive-strong);
		--card-border: var(--positive-strong);
		color: var(--paper);
	}

	.stats__remaining--negative {
		--card-bg: var(--negative-strong);
		--card-border: var(--negative-strong);
	}

	.stat__title {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--neutral-strong);
	}

	/* the hint only shows up on a mismatch, so it wears the negative tone.
	 * .tooltip hard-codes its border to --ink, hence the override. */
	.stats__tooltip {
		--tooltip-bg: var(--negative-subtle);
		--tooltip-fg: var(--negative-strong);
		border-color: var(--negative-strong);
	}

	/* bare icon trigger — a bordered .btn would out-weigh the figure it sits beside */
	.stats__hint {
		padding: 0;
		border: 0;
		background: none;
		color: var(--neutral-strong);
		cursor: pointer;
		vertical-align: -0.125em;
	}

	.stats__hint:hover,
	.stats__hint:focus-visible {
		color: var(--ink);
	}

	/* tone is opt-in, so a hint only alarms where something is actually off */
	.stats__hint--negative,
	.stats__hint--negative:hover,
	.stats__hint--negative:focus-visible {
		color: var(--negative-strong);
	}

	.stat__value {
		font-variant-numeric: tabular-nums;
	}

	.stats__split {
		margin-block-start: var(--space-4);
	}

	/* the card fill already carries the tone, so the badge outlines in the
	 * card's own text colour rather than adding a second green/red */
	.stats__status {
		margin-inline-end: auto;
		--badge-border: currentColor;
		--badge-fg: currentColor;
	}

	/* over budget the badge goes solid — spelled out rather than left to
	 * currentColor, which would resolve against the badge's own flipped text colour */
	.stats__status.badge--solid {
		--badge-border: var(--paper);
		--badge-bg: var(--paper);
		--badge-fg: var(--negative-strong);
	}

	/* stands in for the badge dot when over budget, in the dot's own box so the
	 * label sits in the same place in both states */
	.stats__status-mark {
		inline-size: var(--space-2);
		margin-inline-end: var(--space-1);
		line-height: 1;
		text-align: center;
	}

	/* on the solid card the muted neutrals would sink into the fill.
	 * the row is a flex line so the status badge can sit at the far edge. */
	.stats__remaining .stat__title {
		color: inherit;
	}

	.stats__remaining .stats__hint,
	.stats__remaining .stats__hint:hover,
	.stats__remaining .stats__hint:focus-visible {
		display: flex;
		align-items: center;
		flex-shrink: 0;
		color: inherit;
	}

	section {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		margin-block-start: var(--space-8);

		/* each list is short enough to read whole — no vertical scroll region */
		--table-scroll-max-h: none;
	}

	/* The page is the scroller, so the head and totals stick to the viewport.
	   `clip` still trims the table to the card's rounded corners but — unlike
	   `auto` — is not a scroll container, which is what sticky binds to. */
	@media (width > 48rem) {
		.table-scroll {
			overflow: clip;
		}
	}

	/* fixed columns so the tables line up with each other */
	.category {
		min-inline-size: 200px;
	}
	.amount {
		inline-size: 140px;
	}

	.date {
		color: var(--neutral-strong);
		font-variant-numeric: tabular-nums;
	}

	.hint {
		color: var(--neutral-strong);
		font-size: var(--font-size-1);
	}

	/* money coming back out of an expense column */
	.credit {
		color: var(--positive-strong);
	}

	/* the total row is already a solid fill — the green would sink into it */
	.solid .credit {
		color: inherit;
	}
</style>
