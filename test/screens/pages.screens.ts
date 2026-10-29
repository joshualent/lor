// Seeds realistic data through the admin API and the real `lor` CLI, then
// screenshots every page at desktop and phone widths. Not part of CI:
//   make screens SCREENS_DIR=/some/dir
import { expect, test, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import {
	addAuthenticator,
	addPasskey,
	admin,
	login,
	lor,
	ORIGIN,
	request,
	setup,
	state,
	stepUp
} from './seed';

const out = process.env.LOR_SCREENS_DIR ?? join(import.meta.dirname, '.tmp', 'out');
mkdirSync(out, { recursive: true });

const DESKTOP = { width: 1280, height: 800 };
const PHONE = { width: 390, height: 844 };

test.describe.configure({ mode: 'serial' });
test.setTimeout(10 * 60_000);

let context: BrowserContext;
let page: Page;
let auth: Awaited<ReturnType<typeof addAuthenticator>>;

async function settle(p: Page) {
	await p.waitForLoadState('networkidle').catch(() => {});
	await p.waitForTimeout(400);
}

/** Both widths, full page. `prepare` runs after each resize (e.g. open a dialog). */
async function shoot(
	name: string,
	path: string,
	opts: {
		prepare?: (p: Page) => Promise<void>;
		on?: Page;
		fullPage?: boolean;
		widths?: ('desktop' | 'phone')[];
	} = {}
) {
	const p = opts.on ?? page;
	const widths = opts.widths ?? ['desktop', 'phone'];
	for (const [suffix, size] of [
		['desktop', DESKTOP],
		['phone', PHONE]
	] as const) {
		if (!widths.includes(suffix)) continue;
		await p.setViewportSize(size);
		await p.goto(path);
		await settle(p);
		if (opts.prepare) {
			await opts.prepare(p);
			await p.waitForTimeout(500);
		}
		await p.screenshot({
			path: join(out, `${name}-${suffix}.png`),
			fullPage: opts.fullPage ?? true
		});
	}
	await p.setViewportSize(DESKTOP);
}

interface Ids {
	[k: string]: number;
}
const ids: Ids = {};

test.beforeAll(async ({ browser }) => {
	context = await browser.newContext({ baseURL: ORIGIN });
	page = await context.newPage();
	auth = await addAuthenticator(page);
});

test.afterAll(async () => {
	await context.close();
});

test('setup, login and empty states', async ({ browser }) => {
	await shoot('setup', '/setup');
	await page.goto('/setup');
	await setup(page, state.setupCode, 'iPhone');

	await shoot('dashboard-empty', '/');
	await shoot('hosts-empty', '/hosts');
	await shoot('agents-empty', '/agents');

	await shootLogin(browser);
});

async function shootLogin(browser: Browser) {
	const anon = await browser.newContext({ baseURL: ORIGIN });
	const p = await anon.newPage();
	await shoot('login', '/login', { on: p });
	await shoot('login-deeplink', '/login?next=%2Fr%2F1', { on: p });
	await anon.close();
}

test('seed hosts and agents', async () => {
	const [a, b, c, d, e] = state.sshAddrs;
	const hosts = [
		{ name: 'pve', address: a, max_level: 'sudo', hypervisor: true, notes: 'Proxmox VE 9 on the rack server.' },
		{ name: 'nas', address: b, max_level: 'sudo', notes: 'TrueNAS SCALE. ZFS pool "tank".' },
		{ name: 'caddy', address: c, max_level: 'sudo', lor_infra: true, notes: 'Reverse proxy for every web UI.' },
		{ name: 'media', address: d, max_level: 'user' },
		{ name: 'backup', address: e, max_level: 'sudo' },
		{ name: 'docker-vm', address: '127.0.0.1:1', max_level: 'sudo' }
	];
	for (const h of hosts) {
		await admin(page, 'POST', '/hosts', { hypervisor: false, lor_infra: false, notes: '', ...h });
	}
	await admin(page, 'PATCH', '/hosts/backup', { disabled: true });

	for (const [name, addrs] of [
		['claude', ['127.0.0.1']],
		['codex', ['127.0.0.1']],
		['kilo', ['192.168.1.60']],
		['old-bot', ['192.168.1.61', '192.168.1.62']]
	] as const) {
		const en = await admin<{ command: string; agent: { id: number } }>(page, 'POST', '/agents', {
			name,
			lan_addrs: addrs
		});
		ids[`agent-${name}`] = en.agent.id;
		if (name === 'claude' || name === 'codex') {
			const res = await lor(name, ...en.command.split(/\s+/).slice(1));
			expect(res.code, res.stderr).toBe(0);
		}
	}
	await admin(page, 'POST', `/agents/${ids['agent-old-bot']}/status`, { status: 'disabled' });
});

async function authorize(id: number, level: 'user' | 'sudo', duration_s: number) {
	await stepUp(page, {
		kind: 'authorize_request',
		grant_id: id,
		level,
		duration_s,
		confirm_sudo: level === 'sudo'
	});
}

test('seed grants, commands, offers and pending requests', async () => {
	// Active, standard user, with a command log.
	ids.media = await request(
		'claude',
		'media',
		'--for',
		'1h',
		'--reason',
		'Jellyfin stopped transcoding after the update. I need to read its logs and check the GPU device permissions.'
	);
	await authorize(ids.media, 'user', 3600);
	for (const cmd of [
		['journalctl', '-u', 'jellyfin', '--since', '1 hour ago', '--no-pager'],
		['ls', '-l', '/dev/dri'],
		['id'],
		[]
	]) {
		await lor('claude', 'ssh', 'media', '--', ...cmd);
	}

	// Active sudo on a plain host, with commands.
	ids.nas = await request(
		'codex',
		'nas',
		'--sudo',
		'--for',
		'2h',
		'--reason',
		'ZFS pool reports a degraded disk. I need to run zpool status and check SMART data to identify it.'
	);
	await authorize(ids.nas, 'sudo', 2 * 3600);
	for (const cmd of [
		['zpool', 'status', '-v'],
		['smartctl', '-a', '/dev/sdb'],
		['sh', '-c', 'dmesg | grep -i "ata3" | tail -n 50']
	]) {
		await lor('codex', 'ssh', 'nas', '--', ...cmd);
	}

	// Active, expiring soon.
	ids.nasUser = await request(
		'claude',
		'nas',
		'--for',
		'30m',
		'--reason',
		'Check free space on the tank dataset before the nightly snapshot.'
	);
	await authorize(ids.nasUser, 'user', 4 * 60);
	await lor('claude', 'ssh', 'nas', '--', 'df', '-h', '/mnt/tank');

	// A pending extension on the media grant.
	const ext = await lor(
		'claude',
		'extend',
		'media',
		'--for',
		'1h',
		'--reason',
		'The transcode test takes longer than expected; I need another hour to confirm the fix.'
	);
	expect(ext.code, ext.stdout + ext.stderr).toBe(10);
	ids.ext = Number(ext.stdout.match(/\/e\/(\d+)/)![1]);

	// An offer (admin pre-approval).
	const offer = await stepUp(page, {
		kind: 'create_offer',
		agent_id: ids['agent-codex'],
		host_id: (await admin<{ host: { id: number } }>(page, 'GET', '/hosts/pve')).host.id,
		level: 'user',
		duration_s: 30 * 60,
		confirm_sudo: false,
		note: 'For the VM backup check tonight.'
	});
	ids.offer = offer.grant.id;

	// Ended: denied, revoked, released.
	ids.denied = await request(
		'claude',
		'pve',
		'--for',
		'30m',
		'--reason',
		'List the running VMs to see which one is using the most memory.'
	);
	await admin(page, 'POST', `/grants/${ids.denied}/deny`, { note: 'Use the Proxmox UI for this.' });

	ids.revoked = await request(
		'codex',
		'media',
		'--for',
		'1h',
		'--reason',
		'Rebuild the Jellyfin library index after the folder move.'
	);
	await authorize(ids.revoked, 'user', 3600);
	await lor('codex', 'ssh', 'media', '--', 'ls', '/srv/media');
	await admin(page, 'POST', `/grants/${ids.revoked}/revoke`, { note: 'Done for today.' });

	ids.released = await request(
		'claude',
		'caddy',
		'--for',
		'15m',
		'--reason',
		'Reload Caddy after the certificate renewal failed this morning.'
	);
	await authorize(ids.released, 'user', 15 * 60);
	await lor('claude', 'ssh', 'caddy', '--', 'systemctl', 'status', 'caddy');
	await lor('claude', 'release', 'caddy');

	// Pending requests.
	ids.pve = await request(
		'claude',
		'pve',
		'--sudo',
		'--for',
		'2h',
		'--reason',
		'A container on pve keeps restarting. I need sudo to read its config, check pct status and restart it once I find the cause.'
	);
	ids.caddy = await request(
		'codex',
		'caddy',
		'--sudo',
		'--for',
		'1h',
		'--reason',
		'Renew the TLS certificate manually: the ACME challenge keeps failing and I need to edit the Caddyfile.'
	);
	ids.mediaPending = await request(
		'codex',
		'media',
		'--for',
		'1h',
		'--reason',
		'Read the syslog to find why the backup job failed last night.'
	);

	// A second passkey and a second session (another "device").
	await addPasskey(page, 'MacBook');
});

test('second session from a phone', async ({ browser }) => {
	const phone = await browser.newContext({
		baseURL: ORIGIN,
		userAgent:
			'Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Mobile/15E148 Safari/604.1'
	});
	const p = await phone.newPage();
	const { credentials } = await auth.cdp.send('WebAuthn.getCredentials', {
		authenticatorId: auth.authenticatorId
	});
	const other = await addAuthenticator(p);
	for (const credential of credentials) {
		await other.cdp.send('WebAuthn.addCredential', {
			authenticatorId: other.authenticatorId,
			credential
		});
	}
	await p.goto('/login');
	await login(p);
	await phone.close();
});

test('screens', async () => {
	await shoot('dashboard', '/');
	await shoot('consent-sudo-infra', `/r/${ids.caddy}`);
	await shoot('consent-sudo-hypervisor', `/r/${ids.pve}`);
	await shoot('consent-user', `/r/${ids.mediaPending}`);
	// What a phone shows first: the sticky action bar in place.
	await shoot('consent-sudo-infra-viewport', `/r/${ids.caddy}`, {
		fullPage: false,
		widths: ['desktop', 'phone']
	});
	await shoot('consent-user-viewport', `/r/${ids.mediaPending}`, {
		fullPage: false,
		widths: ['desktop', 'phone']
	});
	await shoot('consent-deny', `/r/${ids.mediaPending}`, {
		fullPage: false,
		prepare: async (p) => {
			await p.getByRole('button', { name: 'Deny', exact: true }).first().click();
		}
	});
	await shoot('extension', `/e/${ids.ext}`);
	await shoot('grant-active-sudo', `/grants/${ids.nas}`);
	await shoot('grant-active-user', `/grants/${ids.media}`);
	await shoot('grant-expiring', `/grants/${ids.nasUser}`);
	await shoot('grant-extend', `/grants/${ids.nas}`, {
		fullPage: false,
		prepare: async (p) => {
			await p.getByRole('button', { name: 'Extend', exact: true }).first().click();
		}
	});
	await shoot('grant-offer', `/grants/${ids.offer}`);
	await shoot('grant-denied', `/grants/${ids.denied}`);
	await shoot('grant-revoked', `/grants/${ids.revoked}`);
	await shoot('offer-new', '/grants/new');
	await shoot('hosts', '/hosts');
	await shoot('host-nas', '/hosts/nas');
	await shoot('host-caddy-infra', '/hosts/caddy');
	await shoot('host-no-key', '/hosts/docker-vm');
	await shoot('agents', '/agents');
	await shoot('agents-sheet', '/agents', {
		fullPage: false,
		prepare: async (p) => {
			await p.getByRole('button', { name: /claude/ }).first().click();
		}
	});
	await shoot('agents-disable', '/agents', {
		fullPage: false,
		prepare: async (p) => {
			await p.getByRole('button', { name: /claude/ }).first().click();
			await p.getByRole('button', { name: /Disable Ends/ }).click();
			await expect(p.getByRole('heading', { name: 'Disable claude?' })).toBeVisible();
		}
	});
	await shoot('hosts-add', '/hosts', {
		fullPage: false,
		prepare: async (p) => {
			await p.getByRole('button', { name: 'Add host' }).first().click();
		}
	});
	await shoot('host-edit', '/hosts/pve', {
		prepare: async (p) => {
			await p.getByRole('button', { name: 'Edit' }).click();
		}
	});
	await shoot('audit', '/audit');
	await shoot('audit-filtered', '/audit?action=grant.', {
		prepare: async (p) => {
			await p.locator('[data-action="grant.denied"]').first().getByRole('button').click();
		}
	});
	await shoot('settings', '/settings');
	await shoot('error-grant-404', '/grants/99999');
	await shoot('error-host-404', '/hosts/nope');
	await shoot('loading-dashboard', '/', {
		fullPage: false,
		prepare: async (p) => {
			await p.unroute('**/api/admin/v1/dashboard');
			await p.route('**/api/admin/v1/dashboard', async (r) => {
				await new Promise((ok) => setTimeout(ok, 4000));
				await r.continue().catch(() => {});
			});
			await p.reload();
			await p.waitForTimeout(700);
		}
	});
	await page.unroute('**/api/admin/v1/dashboard');
	if (process.env.LOR_SCREENS_GALLERY !== '0') {
		const res = await page.goto('/gallery');
		if (res?.ok()) await shoot('gallery', '/gallery');
	}
});

// Extra layout evidence for independent reviews: narrow phone, tablet and
// wide desktop. Keep this beside the seeded screen suite so it uses the
// same realistic data and does not depend on a live environment.
test('responsive checkpoints', async () => {
	for (const width of [320, 768, 1440]) {
		await page.setViewportSize({ width, height: 900 });
		for (const [name, path] of [
			['dashboard', '/'],
			['hosts', '/hosts'],
			['agents', '/agents'],
			['settings', '/settings'],
			['audit', '/audit'],
			['consent', `/r/${ids.mediaPending}`]
		]) {
			await page.goto(path);
			await settle(page);
			const overflow = await page.evaluate(
				() => document.documentElement.scrollWidth - document.documentElement.clientWidth
			);
			expect(overflow, `${name} at ${width}px`).toBeLessThanOrEqual(0);
			await page.screenshot({
				path: join(out, `${name}-${width}.png`),
				fullPage: true
			});
		}
	}
});

test('grant identity and consent focus remain clear in compact windows', async () => {
	await page.setViewportSize(DESKTOP);
	await page.goto('/');
	const sudo = page.getByRole('button', { name: `Extend nas for codex, sudo, grant #${ids.nas}`, exact: true });
	const user = page.getByRole('button', { name: `Extend nas for claude, standard user, grant #${ids.nasUser}`, exact: true });
	await expect(sudo).toHaveCount(1);
	await expect(user).toHaveCount(1);
	await expect(page.locator(`a[href="/grants/${ids.nas}"]`).first()).toHaveAccessibleName(/remaining/);
	await sudo.click();
	await expect(page.getByText(`codex · sudo (root) · Grant #${ids.nas}`, { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	for (const size of [{ width: 320, height: 568 }, { width: 390, height: 620 }, PHONE]) {
		await page.setViewportSize(size);
		await page.goto(`/r/${ids.mediaPending}`);
		await page.getByRole('link', { name: 'Change access', exact: true }).click();
		const radio = page.getByRole('radio', { name: /Standard user/ });
		await radio.focus();
		const card = page.locator('label').filter({ has: radio });
		await expect(card).toBeInViewport();
		const box = await card.boundingBox();
		const navigation = await page.locator('[data-slot="app-navigation"]').boundingBox();
		const bar = page.locator('[data-slot="action-bar"]');
		const position = await bar.evaluate((el) => getComputedStyle(el).position);
		const actions = await bar.boundingBox();
		expect(box!.y).toBeGreaterThanOrEqual(navigation!.y + navigation!.height);
		expect(box!.y + box!.height).toBeLessThanOrEqual(position === 'sticky' ? actions!.y : size.height);
		await page.screenshot({ path: join(out, `consent-focused-${size.width}-${size.height}.png`) });
	}
});

test('full reasons and compact decision jumps are reachable', async () => {
	const reason = Array.from({ length: 12 }, (_, index) => `Step ${index + 1}: I will inspect the service logs and identify the failed renewal before making a change.`).join('\n') + '\nEnd of the agent’s reason.';
	await page.route(`**/api/admin/v1/grants/${ids.caddy}`, async (route) => {
		const response = await route.fetch();
		const data = await response.json();
		await route.fulfill({ response, json: { ...data, reason } });
	});
	for (const size of [{ width: 320, height: 568 }, { width: 390, height: 620 }, PHONE]) {
		await page.setViewportSize(size);
		await page.goto(`/r/${ids.caddy}`);
		const read = page.getByRole('link', { name: 'Read full reason', exact: true });
		await expect(read).toBeInViewport();
		await read.click();
		const region = page.getByRole('region', { name: 'Full agent reason, unverified' });
		await expect(region).toBeFocused();
		const block = region.getByTestId('unverified-text');
		await expect(block).toHaveText(reason);
		await page.screenshot({ path: join(out, `consent-reason-start-${size.width}-${size.height}.png`) });
		await block.evaluate((el) => el.scrollIntoView({ block: 'end' }));
		const bar = page.locator('[data-slot="action-bar"]');
		const position = await bar.evaluate((el) => getComputedStyle(el).position);
		const end = (await block.boundingBox())!;
		const bottom = position === 'sticky' ? (await bar.boundingBox())!.y : size.height;
		expect(end.y + end.height).toBeLessThanOrEqual(bottom);
		await page.screenshot({ path: join(out, `consent-reason-end-${size.width}-${size.height}.png`) });
		if (position === 'static') {
			await page.getByRole('link', { name: 'Go to decision', exact: true }).click();
			await expect(page.getByRole('group', { name: 'Decision', exact: true })).toBeFocused();
			await expect(page.getByTestId('authorize')).toBeInViewport();
			await page.screenshot({ path: join(out, `consent-decision-jump-${size.width}-${size.height}.png`) });
		}
		await expect(page.getByTestId('authorize')).toBeDisabled();
		await expect(page.getByRole('checkbox')).not.toBeChecked();
	}
	await page.unroute(`**/api/admin/v1/grants/${ids.caddy}`);
});
