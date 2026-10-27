// Screenshot harness (not part of CI): starts fake SSH servers, `lor-server
// serve --dev` and the Vite dev server (so the dev-only /gallery route
// works), and records their details for seed.screens.ts.
import { spawn, type ChildProcess } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '../..');
export const statePath = join(import.meta.dirname, '.tmp', 'state.json');

function start(
	cmd: string,
	args: string[],
	want: (out: string, vars: Record<string, string>) => boolean,
	opts: { cwd?: string; env?: NodeJS.ProcessEnv } = {}
): Promise<{ proc: ChildProcess; vars: Record<string, string> }> {
	return new Promise((ok, fail) => {
		const proc = spawn(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts });
		const vars: Record<string, string> = {};
		let out = '';
		const timer = setTimeout(() => fail(new Error(`${cmd} did not start:\n${out}`)), 60_000);
		const onData = (b: Buffer) => {
			out += b.toString();
			for (const line of b.toString().split('\n')) {
				const m = line.match(/^([A-Z_]+)=(.*)$/);
				if (m) vars[m[1]] = m[2].trim();
			}
			if (want(out, vars)) {
				clearTimeout(timer);
				ok({ proc, vars });
			}
		};
		proc.stdout!.on('data', onData);
		proc.stderr!.on('data', (b: Buffer) => {
			out += b.toString();
			if (process.env.LOR_SCREENS_DEBUG) process.stderr.write(b);
		});
		proc.on('exit', (code) => fail(new Error(`${cmd} exited ${code}:\n${out}`)));
	});
}

export default async function globalSetup() {
	const bin = join(root, 'bin');
	const data = mkdtempSync(join(tmpdir(), 'lor-screens-'));
	const procs: ChildProcess[] = [];
	const ssh: string[] = [];
	for (let i = 0; i < 5; i++) {
		const s = await start(join(bin, 'fakessh'), ['-listen', '127.0.0.1:0'], (_, v) => !!v.FAKESSH_ADDR);
		procs.push(s.proc);
		ssh.push(s.vars.FAKESSH_ADDR);
	}
	const lor = await start(
		join(bin, 'lor-server'),
		[
			'serve',
			'--dev',
			'--dev-data-dir',
			data,
			'--dev-admin-listen',
			'127.0.0.1:18544',
			'--dev-agent-listen',
			'127.0.0.1:18543',
			'--dev-origin',
			'http://localhost:15173'
		],
		(_, v) => !!v.LOR_DEV_SETUP_CODE && !!v.LOR_DEV_TLS_FINGERPRINT
	);
	procs.push(lor.proc);
	const vite = await start(
		'pnpm',
		['exec', 'vite', 'dev', '--port', '15173', '--strictPort'],
		(out) => out.includes('15173'),
		{ cwd: join(root, 'web'), env: { ...process.env, LOR_DEV_SERVER: 'http://127.0.0.1:18544' } }
	);
	procs.push(vite.proc);
	mkdirSync(join(import.meta.dirname, '.tmp'), { recursive: true });
	writeFileSync(
		statePath,
		JSON.stringify({
			setupCode: lor.vars.LOR_DEV_SETUP_CODE,
			fingerprint: lor.vars.LOR_DEV_TLS_FINGERPRINT,
			sshAddrs: ssh,
			lorBin: join(bin, 'lor'),
			cliRoot: mkdtempSync(join(tmpdir(), 'lor-screens-cli-'))
		})
	);
	return async () => {
		for (const p of procs) p.kill();
	};
}
