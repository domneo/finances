import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const [categoriesRes, accountsRes, ownersRes] = await Promise.all([
		fetch('/api/categories'),
		fetch('/api/accounts'),
		fetch('/api/owners')
	]);

	return {
		categories: (await categoriesRes.json()) as {
			name: string;
			cadence: string;
		}[],
		accounts: (await accountsRes.json()) as string[],
		owners: (await ownersRes.json()) as string[]
	};
};
