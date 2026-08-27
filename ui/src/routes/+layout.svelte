<script lang="ts">
	import { dev } from '$app/environment';
	import { injectAnalytics } from '@vercel/analytics/sveltekit';

	injectAnalytics({ mode: dev ? 'development' : 'production' });

	import favicon from '$lib/assets/favicon.svg';
	import fontsCss from '$lib/assets/ds/fonts.css?url';
	import themesCss from '$lib/assets/ds/themes.css?url';
	import baseCss from '$lib/assets/ds/base.css?url';
	import componentsCss from '$lib/assets/ds/components.css?url';
	import runtimeJs from '$lib/assets/ds/runtime.js?url';
	import iconSprite from '$lib/assets/ds/icons.svg?raw';

	let { children } = $props();
</script>

<svelte:head>
	<title>Finances</title>
	<link rel="icon" href={favicon} />
	<link rel="stylesheet" href={fontsCss} />
	<link rel="stylesheet" href={themesCss} />
	<link rel="stylesheet" href={baseCss} />
	<link rel="stylesheet" href={componentsCss} />
	<script src={runtimeJs} defer></script>
</svelte:head>

<!-- eslint-disable-next-line svelte/no-at-html-tags -->
{@html iconSprite}

{@render children()}

<style>
	:global(body) {
		overflow-x: hidden;
		padding: var(--space-6) var(--space-4);

		@media (min-width: 32rem) {
			padding: var(--space-7) var(--space-5);
		}
	}
</style>
