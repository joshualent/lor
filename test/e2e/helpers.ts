import { execFile } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type APIRequestContext, type Page } from '@playwright/test';

export interface State {
	setupCode: string;
	fingerprint: string;
	agentURL: string;
	sshAddr: string;
	lorBin: string;
	cliDir: string;
}

export const state: State = JSON.parse(readFileSync(join(import.meta.dirname, '.tmp', 'state.json'), 'utf8'));
export const ORIGIN = 'http://localhost:18444';

/** Adds Chrome's virtual authenticator (passkey with user verification). */
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

/** Admin API call with the CSRF headers (shares the page's cookies). */
export async function adminPost(req: APIRequestContext, path: string, data: unknown) {
	const res = await req.post(`/api/admin/v1${path}`, {
		headers: { Origin: ORIGIN, 'X-Lor-Request': '1', 'Content-Type': 'application/json' },
		data
	});
	expect(res.ok(), `${path}: ${res.status()} ${await res.text()}`).toBeTruthy();
	return res.json();
}

export interface CLIResult {
	code: number;
	stdout: string;
	stderr: string;
}

/** Runs the real `lor` CLI with its own config dir. */
export function lor(...args: string[]): Promise<CLIResult> {
	return new Promise((resolve) => {
		execFile(
			state.lorBin,
			args,
			{ env: { ...process.env, LOR_CONFIG_DIR: state.cliDir }, timeout: 30_000 },
			(err, stdout, stderr) => {
				const code = err ? ((err as { code?: number }).code ?? 1) : 0;
				resolve({ code: typeof code === 'number' ? code : 1, stdout, stderr });
			}
		);
	});
}
