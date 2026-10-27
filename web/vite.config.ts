import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import type { Plugin } from 'vite';

// The component gallery (/gallery) is for development only: production
// builds compile an empty page in its place (and its load() returns 404), so
// none of it ships.
function devOnlyGallery(): Plugin {
	return {
		name: 'lor:dev-only-gallery',
		enforce: 'pre',
		apply: 'build',
		load(id) {
			if (id.split('?')[0].replace(/\\/g, '/').endsWith('/src/routes/gallery/+page.svelte')) {
				return '<!-- The component gallery is only built in development. -->';
			}
		}
	};
}

// Where `lor-server --dev` listens (make dev).
const devServer = process.env.LOR_DEV_SERVER ?? 'http://127.0.0.1:8444';

export default defineConfig({
	plugins: [
		devOnlyGallery(),
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Single-page app: every unknown path gets index.html (lor-server does
			// the same), and the client-side router takes over.
			adapter: adapter({ fallback: 'index.html', strict: true }),
			// Hash-mode CSP written into index.html as a <meta> tag. lor-server
			// adds frame-ancestors 'none' as a header (meta tags can't).
			csp: {
				mode: 'hash',
				directives: {
					'default-src': ['self'],
					'script-src': ['self'],
					'style-src': ['self', 'unsafe-inline'],
					'img-src': ['self', 'data:'],
					'font-src': ['self'],
					'connect-src': ['self'],
					'object-src': ['none'],
					'base-uri': ['none'],
					'form-action': ['self']
				}
			}
		})
	],
	server: {
		proxy: {
			'/api': { target: devServer, changeOrigin: false },
			'/bootstrap': { target: devServer, changeOrigin: false }
		}
	},
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'unit',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
