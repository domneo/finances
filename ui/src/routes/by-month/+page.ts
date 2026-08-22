import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	let month = url.searchParams.get('m') ?? '';

	if (!month) {
		const latest = await fetch('/api/transactions?limit=1');
		const json = (await latest.json()) as { data: { date: string }[] };
		month = json.data[0]?.date?.slice(0, 7) ?? '';
	}

	if (!month) {
		const [categoriesRes, accountsRes] = await Promise.all([
			fetch('/api/categories'),
			fetch('/api/accounts')
		]);
		return {
			month,
			account: '',
			category: '',
			transactions: [],
			categories: (await categoriesRes.json()) as { name: string; cadence: string }[],
			accounts: (await accountsRes.json()) as string[],
			summary: [] as { category: string | null; total: number }[],
			yearlySummary: [] as { category: string | null; total: number }[]
		};
	}

	const [year, mon] = month.split('-');
	const lastDay = new Date(Number(year), Number(mon), 0).getDate();
	const from = `${month}-01`;
	const to = `${month}-${String(lastDay).padStart(2, '0')}`;

	const account = url.searchParams.get('account') ?? '';
	const category = url.searchParams.get('category') ?? '';

	const params = new URLSearchParams({ from, to, limit: '500' });
	if (account) params.set('account', account);
	if (category) params.set('category', category);

	const summaryParams = new URLSearchParams({ from, to });
	if (account) summaryParams.set('account', account);

	const today = new Date();
	const twelveMonthsAgo = new Date(today);
	twelveMonthsAgo.setFullYear(today.getFullYear() - 1);
	const yearlyParams = new URLSearchParams({
		from: twelveMonthsAgo.toISOString().slice(0, 10),
		to: today.toISOString().slice(0, 10)
	});
	if (account) yearlyParams.set('account', account);

	const [txRes, categoriesRes, accountsRes, summaryRes, yearlySummaryRes] = await Promise.all([
		fetch(`/api/transactions?${params}`),
		fetch('/api/categories'),
		fetch('/api/accounts'),
		fetch(`/api/transactions/summary?${summaryParams}`),
		fetch(`/api/transactions/summary?${yearlyParams}`)
	]);
	const json = (await txRes.json()) as { data: Record<string, unknown>[] };

	return {
		month,
		account,
		category,
		transactions: json.data,
		categories: (await categoriesRes.json()) as { name: string; cadence: string }[],
		accounts: (await accountsRes.json()) as string[],
		summary: (await summaryRes.json()) as { category: string | null; total: number }[],
		yearlySummary: (await yearlySummaryRes.json()) as { category: string | null; total: number }[]
	};
};
