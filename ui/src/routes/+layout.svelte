<script lang="ts">
	import favicon from '$lib/assets/favicon.svg';
	import iconSprite from '../../static/ds/icons.svg?raw';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
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
