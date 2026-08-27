import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			'/api': 'http://localhost:5001'
		},
		fs: {
			// src/lib/assets/ds may be a symlink to ../bleed/dist (a sibling
			// design-system repo); serving through it needs the target allowed.
			// Harmless when the sibling isn't there and sync-ds.js copied the
			// package in instead.
			allow: ['.', new URL('../../bleed/dist', import.meta.url).pathname]
		}
	}
});
