import type { PageLoad } from './$types';

const LIMIT = 50;

export const load: PageLoad = async ({ fetch, url }) => {
	const p = url.searchParams;
	const txParams = new URLSearchParams();

	for (const key of ['account', 'category', 'owner', 'from', 'to']) {
		const v = p.get(key);
		if (v) txParams.set(key, v);
	}
	const offset = Number(p.get('offset') ?? 0);
	txParams.set('limit', String(LIMIT));
	txParams.set('offset', String(offset));

	const today = new Date();
	const twelveMonthsAgo = new Date(today);
	twelveMonthsAgo.setFullYear(today.getFullYear() - 1);
	const summaryParams = new URLSearchParams({
		from: twelveMonthsAgo.toISOString().slice(0, 10),
		to: today.toISOString().slice(0, 10)
	});
	// The category totals follow the owner filter, so picking a partner turns
	// the chart into their personal budget.
	const owner = p.get('owner');
	if (owner) summaryParams.set('owner', owner);

	const [txRes, categoriesRes, accountsRes, ownersRes, summaryRes] = await Promise.all([
		fetch(`/api/transactions?${txParams}`),
		fetch('/api/categories'),
		fetch('/api/accounts'),
		fetch('/api/owners'),
		fetch(`/api/transactions/summary?${summaryParams}`)
	]);

	return {
		transactions: (await txRes.json()) as {
			total: number;
			limit: number;
			offset: number;
			data: Record<string, unknown>[];
		},
		categories: (await categoriesRes.json()) as { name: string; cadence: string }[],
		accounts: (await accountsRes.json()) as string[],
		owners: (await ownersRes.json()) as string[],
		summary: (await summaryRes.json()) as { category: string | null; total: number }[],
		params: Object.fromEntries(p)
	};
};
