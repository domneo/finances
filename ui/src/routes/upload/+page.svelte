<script lang="ts">
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type ImportedTxn = {
		date: string;
		account: string;
		category: string | null;
		reference: string | null;
		amount: number;
		currency: string;
		owner: string | null;
	};

	// Editable review row. category/reference/owner are kept as plain strings
	// here ("" means none) for easy form binding; they are converted back to
	// null on save.
	type Row = {
		_id: number;
		date: string;
		account: string;
		category: string;
		reference: string;
		amount: number;
		currency: string;
		owner: string;
	};

	type FileResult = {
		name: string;
		status: 'pending' | 'done' | 'error';
		count?: number;
		error?: string;
	};

	let files = $state<File[]>([]);
	let fileResults = $state<FileResult[]>([]);
	let parsing = $state(false);

	let rows = $state<Row[]>([]);
	let saving = $state(false);
	let saved = $state<number | null>(null);
	let saveError = $state<string | null>(null);
	let dragging = $state(false);

	let nextId = 0;

	function pick(list: FileList | null) {
		if (!list) return;
		files = Array.from(list);
		fileResults = [];
	}

	async function parse() {
		if (files.length === 0 || parsing) return;
		parsing = true;
		saved = null;
		saveError = null;
		rows = [];
		fileResults = files.map((f) => ({ name: f.name, status: 'pending' }));

		for (let i = 0; i < files.length; i++) {
			const body = new FormData();
			body.append('file', files[i]);
			try {
				const res = await fetch('/api/transactions/import', { method: 'POST', body });
				const result = await res.json().catch(() => ({}));
				if (res.ok) {
					const imported = (result.transactions ?? []) as ImportedTxn[];
					rows.push(
						...imported.map((t) => ({
							_id: nextId++,
							date: t.date,
							account: t.account,
							category: t.category ?? '',
							reference: t.reference ?? '',
							amount: t.amount,
							currency: t.currency,
							owner: t.owner ?? ''
						}))
					);
					fileResults[i] = { name: files[i].name, status: 'done', count: imported.length };
				} else {
					fileResults[i] = {
						name: files[i].name,
						status: 'error',
						error: result.error ?? `HTTP ${res.status}`
					};
				}
			} catch (e) {
				fileResults[i] = { name: files[i].name, status: 'error', error: String(e) };
			}
		}

		parsing = false;
		files = [];
	}

	function removeRow(id: number) {
		rows = rows.filter((r) => r._id !== id);
	}

	async function save() {
		if (rows.length === 0 || saving) return;
		saving = true;
		saveError = null;

		const payload = rows.map((r) => ({
			date: r.date,
			account: r.account,
			category: r.category === '' ? null : r.category,
			reference: r.reference === '' ? null : r.reference,
			amount: r.amount,
			currency: r.currency,
			owner: r.owner === '' ? null : r.owner
		}));

		try {
			const res = await fetch('/api/transactions', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});
			const result = await res.json().catch(() => ({}));
			if (res.ok) {
				saved = result.inserted ?? payload.length;
				rows = [];
				fileResults = [];
			} else {
				saveError = result.error ?? `HTTP ${res.status}`;
			}
		} catch (e) {
			saveError = String(e);
		}

		saving = false;
	}
</script>

<svelte:head>
	<title>Upload statements · Finances</title>
</svelte:head>

<main>
	<h1>Upload statements</h1>
	<p class="hint">
		Drop one or more bank statement files (DBS account/card CSV, UOB card .xls). The format is
		auto-detected and transactions are parsed for review — nothing is saved until you confirm.
	</p>

	<label
		class="dropzone"
		class:dragging
		ondragover={(e) => {
			e.preventDefault();
			dragging = true;
		}}
		ondragleave={() => (dragging = false)}
		ondrop={(e) => {
			e.preventDefault();
			dragging = false;
			pick(e.dataTransfer?.files ?? null);
		}}
	>
		<input type="file" multiple onchange={(e) => pick(e.currentTarget.files)} />
		<span>Drop files here or click to browse</span>
	</label>

	{#if files.length > 0}
		<ul class="files">
			{#each files as f}
				<li>{f.name}</li>
			{/each}
		</ul>
		<button class="btn" onclick={parse} disabled={parsing}>
			{parsing ? 'Parsing…' : `Parse ${files.length} file${files.length > 1 ? 's' : ''}`}
		</button>
	{/if}

	{#if fileResults.length > 0}
		<div class="table-scroll status">
			<table class="table">
				<thead>
					<tr>
						<th>File</th>
						<th>Status</th>
						<th class="num">Parsed</th>
					</tr>
				</thead>
				<tbody>
					{#each fileResults as r}
						<tr>
							<td>{r.name}</td>
							<td>
								{#if r.status === 'pending'}
									<span class="badge">parsing…</span>
								{:else if r.status === 'done'}
									<span class="badge badge--positive">parsed</span>
								{:else}
									<span class="badge badge--negative">{r.error}</span>
								{/if}
							</td>
							<td class="num">{r.count ?? ''}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	{#if saved !== null}
		<div
			class="alert"
			role="status"
			style="--alert-border: var(--positive-strong); --alert-fg: var(--positive-strong);"
		>
			<p class="alert__text">Saved {saved} transaction{saved === 1 ? '' : 's'}.</p>
		</div>
	{/if}

	{#if rows.length > 0}
		<h2>Review ({rows.length})</h2>
		<p class="hint">
			Edit any field before saving. Suggested categories and owners are pre-filled; a row with no
			owner is joint.
		</p>

		<div class="table-scroll">
			<table class="table review">
				<thead>
					<tr>
						<th>Date</th>
						<th>Account</th>
						<th>Category</th>
						<th>Owner</th>
						<th>Reference</th>
						<th class="num">Amount</th>
						<th>Cur</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row._id)}
						<tr>
							<td><input type="date" class="input" bind:value={row.date} /></td>
							<td>
								<input list="accounts" class="input" bind:value={row.account} />
							</td>
							<td>
								<select class="input" bind:value={row.category}>
									<option value="">—</option>
									{#each data.categories as c}
										<option value={c.name}>{c.name}</option>
									{/each}
								</select>
							</td>
							<td>
								<select class="input" bind:value={row.owner}>
									<option value="">Joint</option>
									{#each data.owners as o}
										<option value={o}>{o}</option>
									{/each}
								</select>
							</td>
							<td><input class="input" bind:value={row.reference} /></td>
							<td class="num">
								<input type="number" step="0.01" class="input" bind:value={row.amount} />
							</td>
							<td><input class="input cur" bind:value={row.currency} /></td>
							<td>
								<button
									class="btn btn--icon btn--sm"
									style="--btn-fg: var(--negative-strong); --btn-border: var(--negative-strong);"
									onclick={() => removeRow(row._id)}
									title="Remove">×</button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<datalist id="accounts">
			{#each data.accounts as a}
				<option value={a}></option>
			{/each}
		</datalist>

		{#if saveError}
			<div
				class="alert"
				role="alert"
				style="--alert-border: var(--negative-strong); --alert-fg: var(--negative-strong);"
			>
				<p class="alert__text">{saveError}</p>
			</div>
		{/if}

		<button class="btn btn--solid save" onclick={save} disabled={saving}>
			{saving ? 'Saving…' : `Save ${rows.length} transaction${rows.length === 1 ? '' : 's'}`}
		</button>
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
		margin: 0 0 var(--space-2);
	}

	h2 {
		font-family: var(--font-display);
		font-size: var(--font-size-3);
		margin: var(--space-6) 0 var(--space-1);
	}

	.hint {
		color: var(--neutral-strong);
		margin: 0 0 var(--space-4);
	}

	.dropzone {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-6);
		border: var(--border-w) dashed var(--ink);
		border-radius: var(--radius);
		color: var(--neutral-strong);
		cursor: pointer;
		background: var(--paper);
		transition:
			border-color var(--transition),
			color var(--transition);
	}

	.dropzone.dragging {
		border-style: solid;
		color: var(--ink);
	}

	.dropzone input {
		display: none;
	}

	.files {
		list-style: none;
		padding: 0;
		margin: var(--space-4) 0;
	}

	.files li {
		padding: var(--space-1) 0;
		border-bottom: var(--border-w) var(--border-style)
			color-mix(in oklch, var(--ink) 15%, var(--paper));
	}

	.status {
		margin-top: var(--space-5);
	}

	table.review td {
		padding: var(--space-1) var(--space-2);
	}

	table.review .input {
		min-width: 8rem;
	}

	table.review td.num .input {
		text-align: end;
		min-width: 6rem;
	}

	table.review .cur {
		min-width: 4rem;
		max-width: 4.5rem;
	}

	button.save {
		margin-top: var(--space-4);
	}
</style>
