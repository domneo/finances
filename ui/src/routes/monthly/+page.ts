import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const p = url.searchParams;
	const params = new URLSearchParams();
	if (p.get('from')) params.set('from', p.get('from')!);
	if (p.get('to')) params.set('to', p.get('to')!);

	const res = await fetch(`/api/transactions/monthly?${params}`);
	return {
		monthly: (await res.json()) as { month: string; spend: number; income: number; net: number }[],
		params: Object.fromEntries(p)
	};
};
