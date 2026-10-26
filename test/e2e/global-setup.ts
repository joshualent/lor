// Starts a fake SSH server and `lor-server serve --dev` with a throwaway data
// dir, and records their details for the tests.
import { spawn, type ChildProcess } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '../..');
export const statePath = join(import.meta.dirname, '.tmp', 'state.json');

function start(cmd: string, args: string[], want: string[]): Promise<{ proc: ChildProcess; vars: Record<string, string> }> {
	return new Promise((ok, fail) => {
		const proc = spawn(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'] });
		const vars: Record<string, string> = {};
		let out = '';
		const timer = setTimeout(() => fail(new Error(`${cmd} did not start:\n${out}`)), 20_000);
		const onData = (b: Buffer) => {
			out += b.toString();
			for (const line of b.toString().split('\n')) {
				const m = line.match(/^([A-Z_]+)=(.*)$/);
				if (m) vars[m[1]] = m[2].trim();
			}
			if (want.every((k) => vars[k])) {
				clearTimeout(timer);
				ok({ proc, vars });
			}
		};
		proc.stdout!.on('data', onData);
		proc.stderr!.on('data', (b: Buffer) => {
			out += b.toString();
			if (process.env.LOR_E2E_DEBUG) process.stderr.write(b);
		});
		proc.on('exit', (code) => fail(new Error(`${cmd} exited ${code}:\n${out}`)));
	});
}

export default async function globalSetup() {
	const bin = join(root, 'bin');
	const data = mkdtempSync(join(tmpdir(), 'lor-e2e-'));
	const ssh = await start(join(bin, 'fakessh'), ['-listen', '127.0.0.1:0'], ['FAKESSH_ADDR']);
	const lor = await start(
		join(bin, 'lor-server'),
		['serve', '--dev', '--dev-data-dir', data, '--dev-admin-listen', '127.0.0.1:18444', '--dev-agent-listen', '127.0.0.1:18443'],
		['LOR_DEV_SETUP_CODE', 'LOR_DEV_TLS_FINGERPRINT']
	);
	mkdirSync(join(import.meta.dirname, '.tmp'), { recursive: true });
	writeFileSync(
		statePath,
		JSON.stringify({
			setupCode: lor.vars.LOR_DEV_SETUP_CODE,
			fingerprint: lor.vars.LOR_DEV_TLS_FINGERPRINT,
			agentURL: lor.vars.LOR_DEV_AGENT_URL,
			sshAddr: ssh.vars.FAKESSH_ADDR,
			lorBin: join(bin, 'lor'),
			cliDir: mkdtempSync(join(tmpdir(), 'lor-e2e-cli-'))
		})
	);
	return async () => {
		lor.proc.kill();
		ssh.proc.kill();
	};
}
