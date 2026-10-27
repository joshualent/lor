// Seeding helpers for the screenshot harness: admin API calls, passkey
// ceremonies run inside the page (Chrome's virtual authenticator), and the
// real `lor` CLI as one or more agents.
import { execFile } from 'node:child_process';
import { mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Page } from '@playwright/test';

export interface State {
	setupCode: string;
	fingerprint: string;
	sshAddrs: string[];
	lorBin: string;
	cliRoot: string;
}

export const state: State = JSON.parse(
	readFileSync(join(import.meta.dirname, '.tmp', 'state.json'), 'utf8')
);
export const ORIGIN = 'http://localhost:15173';

export async function addAuthenticator(page: Page) {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('WebAuthn.enable');
	const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: {
			protocol: 'ctap2',
			transport: 'internal',
			hasResidentKey: true,
			hasUserVerification: true,
			isUserVerified: true,
			automaticPresenceSimulation: true
		}
	});
	return { cdp, authenticatorId };
}

/** Admin API call from the page's cookie jar. */
export async function admin<T = unknown>(
	page: Page,
	method: 'GET' | 'POST' | 'PATCH',
	path: string,
	data?: unknown
): Promise<T> {
	const res = await page.request.fetch(`/api/admin/v1${path}`, {
		method,
		headers: { Origin: ORIGIN, 'X-Lor-Request': '1', 'Content-Type': 'application/json' },
		data: method === 'GET' ? undefined : (data ?? {})
	});
	expect(res.ok(), `${method} ${path}: ${res.status()} ${await res.text()}`).toBeTruthy();
	return res.json();
}

/**
 * Runs a WebAuthn ceremony inside the page: begin → navigator.credentials →
 * finish. kind "get" is an assertion (login/step-up), "create" a registration.
 */
async function ceremony(
	page: Page,
	beginPath: string,
	beginBody: unknown,
	finishPath: string,
	kind: 'get' | 'create'
) {
	return page.evaluate(
		async ({ beginPath, beginBody, finishPath, kind }) => {
			const h = { 'X-Lor-Request': '1', 'Content-Type': 'application/json' };
			const b64 = (s: string) => {
				const p = s.replace(/-/g, '+').replace(/_/g, '/');
				const bin = atob(p + '='.repeat((4 - (p.length % 4)) % 4));
				return Uint8Array.from(bin, (c) => c.charCodeAt(0)).buffer;
			};
			const enc = (buf: ArrayBuffer | null) =>
				buf
					? btoa(String.fromCharCode(...new Uint8Array(buf)))
							.replace(/\+/g, '-')
							.replace(/\//g, '_')
							.replace(/=+$/, '')
					: undefined;
			const post = async (path: string, body: unknown) => {
				const r = await fetch(`/api/admin/v1${path}`, {
					method: 'POST',
					headers: h,
					body: JSON.stringify(body)
				});
				if (!r.ok) throw new Error(`${path}: ${r.status} ${await r.text()}`);
				return r.json();
			};
			const begin = await post(beginPath, beginBody);
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const finishOne = async (ceremonyId: string, opts: any, k: 'get' | 'create') => {
				const o = opts.publicKey ?? opts;
				let cred: PublicKeyCredential;
				if (k === 'get') {
					cred = (await navigator.credentials.get({
						publicKey: {
							...o,
							challenge: b64(o.challenge),
							allowCredentials: (o.allowCredentials ?? []).map(
								(c: { id: string; type: 'public-key' }) => ({ ...c, id: b64(c.id) })
							)
						}
					})) as PublicKeyCredential;
					const r = cred.response as AuthenticatorAssertionResponse;
					return post(finishPath, {
						ceremony_id: ceremonyId,
						credential: {
							id: cred.id,
							rawId: enc(cred.rawId),
							type: cred.type,
							response: {
								clientDataJSON: enc(r.clientDataJSON),
								authenticatorData: enc(r.authenticatorData),
								signature: enc(r.signature),
								userHandle: enc(r.userHandle)
							},
							clientExtensionResults: {}
						}
					});
				}
				cred = (await navigator.credentials.create({
					publicKey: {
						...o,
						challenge: b64(o.challenge),
						user: { ...o.user, id: b64(o.user.id) },
						excludeCredentials: (o.excludeCredentials ?? []).map(
							(c: { id: string; type: 'public-key' }) => ({ ...c, id: b64(c.id) })
						)
					}
				})) as PublicKeyCredential;
				const r = cred.response as AuthenticatorAttestationResponse;
				return post(finishPath, {
					ceremony_id: ceremonyId,
					credential: {
						id: cred.id,
						rawId: enc(cred.rawId),
						type: cred.type,
						response: {
							clientDataJSON: enc(r.clientDataJSON),
							attestationObject: enc(r.attestationObject),
							transports: r.getTransports?.() ?? []
						},
						clientExtensionResults: {}
					}
				});
			};
			return finishOne(begin.ceremony_id, begin.options, kind);
		},
		{ beginPath, beginBody, finishPath, kind }
	);
}

export const setup = (page: Page, code: string, name: string) =>
	ceremony(page, '/setup/begin', { code, passkey_name: name }, '/setup/finish', 'create');

export const login = (page: Page) => ceremony(page, '/login/begin', {}, '/login/finish', 'get');

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const stepUp = (page: Page, action: Record<string, unknown>): Promise<any> =>
	ceremony(page, '/stepup/begin', { action }, '/stepup/finish', 'get');

/** add_passkey step-up, then the registration ceremony it returns. */
export async function addPasskey(page: Page, name: string) {
	const res = await stepUp(page, { kind: 'add_passkey', passkey_name: name });
	// The registration is its own ceremony with the returned options.
	return page.evaluate(
		async ({ reg }) => {
			const h = { 'X-Lor-Request': '1', 'Content-Type': 'application/json' };
			const b64 = (s: string) => {
				const p = s.replace(/-/g, '+').replace(/_/g, '/');
				const bin = atob(p + '='.repeat((4 - (p.length % 4)) % 4));
				return Uint8Array.from(bin, (c) => c.charCodeAt(0)).buffer;
			};
			const enc = (buf: ArrayBuffer) =>
				btoa(String.fromCharCode(...new Uint8Array(buf)))
					.replace(/\+/g, '-')
					.replace(/\//g, '_')
					.replace(/=+$/, '');
			const o = reg.options.publicKey ?? reg.options;
			const cred = (await navigator.credentials.create({
				publicKey: {
					...o,
					challenge: b64(o.challenge),
					user: { ...o.user, id: b64(o.user.id) },
					excludeCredentials: []
				}
			})) as PublicKeyCredential;
			const r = cred.response as AuthenticatorAttestationResponse;
			const res = await fetch('/api/admin/v1/passkeys/register/finish', {
				method: 'POST',
				headers: h,
				body: JSON.stringify({
					ceremony_id: reg.ceremony_id,
					credential: {
						id: cred.id,
						rawId: enc(cred.rawId),
						type: cred.type,
						response: {
							clientDataJSON: enc(r.clientDataJSON),
							attestationObject: enc(r.attestationObject),
							transports: r.getTransports?.() ?? []
						},
						clientExtensionResults: {}
					}
				})
			});
			if (!res.ok) throw new Error(`register finish: ${res.status} ${await res.text()}`);
			return res.json();
		},
		{ reg: res.registration }
	);
}

export interface CLIResult {
	code: number;
	stdout: string;
	stderr: string;
}

/** The real `lor` CLI, one config dir per agent name. */
export function lor(agent: string, ...args: string[]): Promise<CLIResult> {
	const dir = join(state.cliRoot, agent);
	mkdirSync(dir, { recursive: true });
	return new Promise((resolve) => {
		execFile(
			state.lorBin,
			args,
			{ env: { ...process.env, LOR_CONFIG_DIR: dir }, timeout: 30_000 },
			(err, stdout, stderr) => {
				const code = err ? ((err as { code?: number }).code ?? 1) : 0;
				resolve({ code: typeof code === 'number' ? code : 1, stdout, stderr });
			}
		);
	});
}

/** `lor request` → the new grant id (exit 10 = pending). */
export async function request(agent: string, host: string, ...args: string[]): Promise<number> {
	const res = await lor(agent, 'request', host, ...args);
	expect(res.code, `${res.stdout}\n${res.stderr}`).toBe(10);
	return Number(res.stdout.match(/\/r\/(\d+)/)![1]);
}
