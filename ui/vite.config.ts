import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			'/api': 'http://localhost:5001'
		},
		fs: {
			// static/bleed is a symlink to ../bleed/dist (a sibling design-system repo)
			allow: ['.', new URL('../../bleed/dist', import.meta.url).pathname]
		}
	}
});
