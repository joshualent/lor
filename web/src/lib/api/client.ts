// Typed admin API client (openapi-fetch over the generated admin.d.ts).
// Adds the CSRF header to every request and sends signed-out users to
// /login?next=<here> on 401.
import createClient, { type Middleware } from 'openapi-fetch';
import { goto } from '$app/navigation';
import type { components, paths } from './admin';
import { ApiError } from './error';

export { ApiError };

export type Schemas = components['schemas'];
export type GrantDetail = Schemas['GrantDetail'];
export type GrantSummary = Schemas['GrantSummary'];
export type Warning = Schemas['Warning'];
export type Level = Schemas['Level'];
export type StepUpAction = Schemas['StepUpAction'];
export type StepUpResult = Schemas['StepUpResult'];

const PUBLIC_PATHS = ['/login', '/setup'];

const csrf: Middleware = {
	onRequest({ request }) {
		request.headers.set('X-Lor-Request', '1');
		if (request.method !== 'GET' && request.method !== 'HEAD') {
			request.headers.set('Content-Type', 'application/json');
		}
		return request;
	},
	async onResponse({ response }) {
		if (response.status === 401 && typeof window !== 'undefined') {
			const here = window.location.pathname + window.location.search;
			if (!PUBLIC_PATHS.some((p) => window.location.pathname.startsWith(p))) {
				await goto(`/login?next=${encodeURIComponent(here)}`, { replaceState: true });
			}
		}
		return response;
	}
};

export const api = createClient<paths>({ baseUrl: '/api/admin/v1', credentials: 'same-origin' });
api.use(csrf);

type Result<T> = { data?: T; error?: unknown; response: Response };

/** Unwraps an openapi-fetch result, throwing ApiError with the server's message. */
export async function call<T>(p: Promise<Result<T>>): Promise<T> {
	let res: Result<T>;
	try {
		res = await p;
	} catch {
		throw new ApiError(0, 'network', "Can't reach Lor. Check your connection and try again.");
	}
	if (res.error !== undefined || !res.response.ok) {
		const e = (res.error ?? {}) as { error?: { code?: string; message?: string } };
		throw new ApiError(
			res.response.status,
			e.error?.code ?? `http_${res.response.status}`,
			e.error?.message ?? `Request failed (HTTP ${res.response.status})`
		);
	}
	return res.data as T;
}
