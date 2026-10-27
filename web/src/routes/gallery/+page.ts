// The component gallery exists only in dev builds (Vite dev server). In the
// production bundle the route always 404s; it holds no data or secrets.
import { error } from '@sveltejs/kit';
import { dev } from '$app/env';

export function load() {
	if (!dev) error(404, 'Not found');
}
