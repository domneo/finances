import type { PageLoad } from './$types';

export type Contribution = {
	name: string;
	expected: number;
	received: number;
	count: number;
	date: string;
};

export type Recurring = {
	category: string;
	cadence: 'monthly' | 'yearly';
	monthly: number;
	actual: number;
	count: number;
	lastPaid: string;
};

export type CategoryTotal = {
	category: string;
	total: number;
	count: number;
};

export type Dashboard = {
	month: string;
	budget: Contribution[];
	recurring: Recurring[];
	variable: CategoryTotal[];
};

export const load: PageLoad = async ({ fetch, url }) => {
	const month = url.searchParams.get('m') ?? '';
	const res = await fetch(`/api/dashboard${month ? `?month=${month}` : ''}`);
	return (await res.json()) as Dashboard;
};
