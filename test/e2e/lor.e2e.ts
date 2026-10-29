// End-to-end: the web UI against `lor-server serve --dev`, with Chrome's
// virtual authenticator and the real `lor` CLI as the agent (plan section 13).
import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { join } from 'node:path';
import { addAuthenticator, lor, state } from './helpers';

const shots = join(import.meta.dirname, '.tmp', 'screens');
const HOSTILE = `<script>window.__pwned=1</script><img src=x onerror="window.__pwned=2"> <b>bold</b> Check the ZFS pool & SMART data.`;

test.describe.configure({ mode: 'serial' });

let context: BrowserContext;
let page: Page;
let requestId = '';

test.beforeAll(async ({ browser }) => {
	context = await browser.newContext({ baseURL: 'http://localhost:18444' });
	page = await context.newPage();
	await addAuthenticator(page);
});

test.afterAll(async () => {
	await context.close();
});

async function signIn() {
	await page.getByRole('button', { name: 'Sign in with passkey' }).click();
}

test('first-passkey setup, then login', async () => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/setup$/);
	await page.screenshot({ path: join(shots, 'setup.png') });
	await page.getByLabel('Setup code').fill(state.setupCode);
	await page.getByLabel('Passkey name').fill('Test passkey');
	await page.getByRole('button', { name: 'Register passkey' }).click();
	await expect(page.getByRole('heading', { name: 'Home' })).toBeVisible();

	await page.getByRole('button', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login/);
	await page.screenshot({ path: join(shots, 'login.png') });
	await signIn();
	await expect(page.getByRole('heading', { name: 'Home' })).toBeVisible();
});

test('add a host and enroll an agent', async () => {
	await page.goto('/hosts');
	await expect(page.getByText('No hosts yet')).toBeVisible(); // empty state
	await page.getByRole('button', { name: 'Add host' }).first().click();
	await page.getByLabel('Name').fill('nas');
	await page.getByLabel('LAN IP').fill(state.sshAddr);
	await page.getByRole('switch', { name: 'Lor infrastructure' }).click();
	await expect(page.getByRole('switch', { name: 'Lor infrastructure' })).toBeChecked();
	await page.getByRole('button', { name: 'Add host' }).last().click();
	await expect(page).toHaveURL(/\/hosts\/nas$/);
	await expect(page.getByText('Lor infra', { exact: true })).toBeVisible();
	await expect(page.getByText(/^SHA256:/)).toBeVisible();
	await expect(page.getByText(/curl -fsSL '.*\/bootstrap\/nas\.sh\?t=/).first()).toBeVisible();
	await page.screenshot({ path: join(shots, 'host.png'), fullPage: true });

	await page.goto('/agents');
	await page.getByRole('button', { name: 'Add agent' }).first().click();
	await page.getByLabel('Name').fill('claude');
	await page.getByLabel('LAN IP').fill('127.0.0.1');
	await page.getByRole('button', { name: 'Add agent' }).last().click();
	const cmd = (await page.locator('code', { hasText: 'lor enroll' }).textContent())!.trim();
	expect(cmd).toContain(`--fingerprint ${state.fingerprint}`);
	const args = cmd.split(/\s+/).slice(1);
	const res = await lor(...args);
	expect(res.code, res.stderr).toBe(0);
	expect(res.stdout).toContain('Enrolled as "claude"');
});

test('agent request with a hostile reason; signed-out deep link goes through login', async () => {
	const res = await lor('request', 'nas', '--sudo', '--for', '2h', '--reason', HOSTILE);
	expect(res.code, res.stderr).toBe(10);
	const m = res.stdout.match(/http:\/\/localhost:18444\/r\/(\d+)/);
	expect(m, res.stdout).not.toBeNull();
	requestId = m![1];

	await context.clearCookies();
	await page.goto(`/r/${requestId}`);
	await expect(page).toHaveURL(new RegExp(`/login\\?next=%2Fr%2F${requestId}$`));
	await signIn();
	await expect(page).toHaveURL(new RegExp(`/r/${requestId}$`));
	await expect(page.getByTestId('consent')).toBeVisible();
	await expect(page.getByRole('heading', { name: /claude wants access to\s+nas/ })).toBeVisible();
});

test('the hostile reason is shown as plain text', async () => {
	const block = page.getByTestId('unverified-text');
	await expect(block).toHaveText(HOSTILE);
	expect(await block.locator('b, img, script').count()).toBe(0);
	expect(await page.evaluate(() => (window as unknown as { __pwned?: number }).__pwned)).toBeUndefined();
	await expect(page.getByText('Reason (written by the agent, unverified)')).toBeVisible();
	// Server-computed warnings: Lor infrastructure + root.
	await expect(page.locator('[data-warning="lor_infra"]')).toBeVisible();
	await expect(page.locator('[data-warning="root_access"]')).toBeVisible();
	await page.screenshot({ path: join(shots, 'consent-desktop.png'), fullPage: true });
});

test('the sudo checkbox gates the authorize button', async () => {
	const authorize = page.getByTestId('authorize');
	await expect(authorize).toHaveText(/Authorize sudo on nas for 15 min/); // lor_infra default
	await expect(authorize).toBeDisabled();
	await page.getByText('I understand this gives root on nas').click();
	await expect(authorize).toBeEnabled();
	await page.getByText('I understand this gives root on nas').click();
	await expect(authorize).toBeDisabled();
});

test('authorize with changes: the grant is active at the lower level', async () => {
	await page.getByText('Standard user', { exact: true }).click();
	await expect(page.getByText('I understand this gives root on nas')).toBeHidden();
	await expect(page.locator('[data-warning="root_access"]')).toBeHidden();
	await page.getByRole('radio', { name: '30 min' }).click();
	const authorize = page.getByTestId('authorize');
	await expect(authorize).toHaveText(/Authorize standard user on nas for 30 min/);
	await authorize.click();
	await expect(page.getByTestId('grant')).toBeVisible();
	await expect(page.locator('[data-status="active"]')).toBeVisible();
	await expect(page.getByText(/standard user\s*\(asked for sudo\)/)).toBeVisible();
	// A decided request never shows an Authorize button.
	await expect(page.getByTestId('authorize')).toHaveCount(0);

	const status = await lor('status');
	expect(status.stdout).toMatch(/nas\s+user\s+active/);
});

test('the command log shows commands after lor ssh reports them', async () => {
	// The fake SSH server rejects the login, but the command is reported first.
	const res = await lor('ssh', 'nas', '--', 'zpool', 'status', '-v');
	expect(res.code).toBe(255);
	await page.reload();
	await expect(page.getByTestId('command-log')).toContainText('zpool status -v');
	await expect(page.getByText("Reported by the agent's CLI.")).toBeVisible();
	await page.screenshot({ path: join(shots, 'grant-active.png'), fullPage: true });

	await page.goto('/');
	await expect(page.getByText('zpool status -v')).toBeVisible();
	await page.screenshot({ path: join(shots, 'dashboard.png'), fullPage: true });
});

test('consent page on a phone-sized viewport', async () => {
	const res = await lor('release', 'nas');
	expect(res.code, res.stderr).toBe(0);
	const req = await lor('request', 'nas', '--for', '1h', '--reason', 'Read the syslog to find why the backup job failed last night.');
	expect(req.code).toBe(10);
	const id = req.stdout.match(/\/r\/(\d+)/)![1];

	await page.setViewportSize({ width: 390, height: 844 });
	await page.goto(`/r/${id}`);
	await expect(page.getByTestId('consent')).toBeVisible();
	const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
	expect(overflow).toBeLessThanOrEqual(0);
	// The action bar sticks to the bottom of the screen, reachable one-handed.
	const authorize = page.getByTestId('authorize');
	await expect(authorize).toBeInViewport();
	await expect(authorize).toBeEnabled(); // user level: no checkbox needed
	await page.screenshot({ path: join(shots, 'consent-phone.png') });
	await page.screenshot({ path: join(shots, 'consent-phone-full.png'), fullPage: true });

	// Deny uses a bottom drawer on phones.
	await page.getByRole('button', { name: 'Deny' }).click();
	await expect(page.getByText('Deny this request?')).toBeVisible();
	await page.screenshot({ path: join(shots, 'deny-drawer-phone.png') });
	await page.getByLabel('Note (optional)').fill('Not tonight');
	await page.getByTestId('confirm-deny').click();
	await expect(page.locator('[data-status="denied"]')).toBeVisible();
	await page.setViewportSize({ width: 1280, height: 720 });
});

test('grant access form (offer) with a chat line to copy', async () => {
	await page.goto('/grants/new');
	await page.getByLabel('Agent').selectOption({ label: 'claude' });
	await page.getByLabel('Host').selectOption({ label: `nas · ${state.sshAddr}` });
	await page.getByText('sudo (root)', { exact: false }).first().click();
	const create = page.getByTestId('create-offer');
	await expect(create).toBeDisabled();
	await page.getByText('I understand this gives root on nas').click();
	await expect(create).toBeEnabled();
	await page.screenshot({ path: join(shots, 'offer-form.png'), fullPage: true });
	await create.click();
	await expect(page.getByText(/You have Lor access to `nas` \(sudo\) until/)).toBeVisible();

	// The agent claims it on first use.
	const res = await lor('ssh', 'nas', '--', 'id');
	expect(res.stderr).toContain('claimed the offer');
});

test('the component gallery is not part of production builds', async () => {
	await page.goto('/gallery');
	await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
	await expect(page.getByText('Component gallery')).toHaveCount(0);
});

test('audit log filters by category and shows event details', async () => {
	await page.goto('/audit');
	await expect(page.getByTestId('audit-log')).toBeVisible();
	await page.getByRole('radio', { name: 'Hosts' }).click();
	await expect(page).toHaveURL(/action=host\./);
	await expect(page.locator('[data-action^="grant."]')).toHaveCount(0);
	const added = page.locator('[data-action="host.created"]').first();
	await added.getByRole('button').click();
	await expect(added.getByText('host.created')).toBeVisible();
	// Arrow keys move the actor filter (radio group).
	await page.getByRole('radio', { name: 'Everyone' }).focus();
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('radio', { name: 'You' })).toBeChecked();
});

test('a failed background refresh keeps the last data on screen', async () => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: 'Recent activity' })).toBeVisible();
	await page.route('**/api/admin/v1/dashboard', (r) => r.abort());
	// The dashboard polls every 5 s and retries network errors before failing.
	await expect(page.getByText("Couldn't refresh.")).toBeVisible({ timeout: 30_000 });
	await expect(page.getByRole('heading', { name: 'Recent activity' })).toBeVisible();
	await page.unroute('**/api/admin/v1/dashboard');
	await page.getByRole('button', { name: 'Retry' }).click();
	await expect(page.getByText("Couldn't refresh.")).toHaveCount(0);
});

test('navigation and inventories remain usable across phone, tablet and desktop widths', async () => {
	for (const width of [320, 390, 768, 1024, 1440]) {
		await page.setViewportSize({ width, height: 900 });
		for (const path of ['/', '/hosts', '/agents', '/audit', '/settings']) {
			await page.goto(path);
			const navigation = page.getByRole('navigation', {
				name: 'Main',
				exact: true
			});
			await expect(navigation).toBeVisible();
			await expect(navigation.locator('[aria-current="page"]')).toHaveCount(1);
			for (const link of await navigation.getByRole('link').all()) {
				await expect(link).toBeInViewport();
				const box = await link.boundingBox();
				expect(box!.height).toBeGreaterThanOrEqual(44);
			}
			const overflow = await page.evaluate(
				() => document.documentElement.scrollWidth - document.documentElement.clientWidth
			);
			expect(overflow, `${path} overflows at ${width}px`).toBeLessThanOrEqual(0);
		}
	}
	await page.goto('/agents');
	await page.getByRole('searchbox', { name: 'Filter agents' }).fill('missing-agent');
	await expect(page.getByText('No matching agents')).toBeVisible();
	await page.getByRole('button', { name: 'Clear Filter agents' }).click();
	await expect(page.getByRole('button', { name: /claude/ })).toBeVisible();
	await page.goto('/hosts');
	await page.getByRole('searchbox', { name: 'Filter hosts' }).fill('missing-host');
	await expect(page.getByText('No matching hosts')).toBeVisible();
	await page.getByRole('button', { name: 'Clear Filter hosts' }).click();
	await expect(page.getByRole('link', { name: /nas/ }).last()).toBeVisible();
	await page.setViewportSize({ width: 1280, height: 720 });
});

test('failed sign-out keeps the session view and lets the admin retry', async () => {
	await page.setViewportSize({ width: 1280, height: 800 });
	await page.goto('/');
	await page.route('**/api/admin/v1/logout', (r) => r.fulfill({ status: 500, json: { error: { code: 'test', message: 'Test sign-out failure' } } }));
	await page.getByRole('button', { name: 'Sign out', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Home', exact: true })).toBeVisible();
	await expect(page.getByText(/Couldn't sign out: Test sign-out failure/)).toBeVisible();
	await page.goto('/settings');
	await page.setViewportSize({ width: 390, height: 844 });
	await page.getByRole('button', { name: 'Sign out', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
	await page.unroute('**/api/admin/v1/logout');
	await page.route('**/api/admin/v1/sessions/revoke-all', (r) => r.fulfill({ status: 500, json: { error: { code: 'test', message: 'Test revoke-all failure' } } }));
	await page.getByRole('button', { name: /^Sign out everywhere/ }).click();
	await expect(page.getByRole('heading', { name: 'Sign out everywhere?' })).toBeVisible();
	await page.getByRole('button', { name: 'Sign out everywhere', exact: true }).click();
	await expect(page.getByText('Test revoke-all failure')).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Sign out everywhere?' })).toBeVisible();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	await page.unroute('**/api/admin/v1/sessions/revoke-all');
});

test('disabling an agent requires confirmation and preserves retry after failure', async () => {
	await page.goto('/agents');
	await page.getByRole('button', { name: /claude/ }).first().click();
	await page.getByRole('button', { name: /^Disable/ }).click();
	await expect(page.getByRole('heading', { name: 'Disable claude?' })).toBeVisible();
	await expect(page.getByText(/Re-enabling it will not restore access/)).toBeVisible();
	await page.route('**/api/admin/v1/agents/*/status', (r) => r.fulfill({ status: 500, json: { error: { code: 'test', message: 'Test disable failure' } } }));
	await page.getByRole('button', { name: 'Disable agent', exact: true }).click();
	await expect(page.getByText('Test disable failure')).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Disable claude?' })).toBeVisible();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	await page.unroute('**/api/admin/v1/agents/*/status');
});

test('confirmation actions stay in view in short desktop and phone windows', async () => {
	for (const viewport of [{ width: 1280, height: 560 }, { width: 390, height: 500 }]) {
		await page.setViewportSize(viewport);
		await page.goto('/hosts');
		await page.getByRole('button', { name: 'Add host', exact: true }).first().click();
		await page.getByRole('switch', { name: 'Lor infrastructure' }).scrollIntoViewIfNeeded();
		await expect(page.getByRole('button', { name: 'Add host', exact: true }).last()).toBeInViewport();
		await expect(page.getByRole('button', { name: 'Cancel', exact: true })).toBeInViewport();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	}
	await page.setViewportSize({ width: 1280, height: 800 });
});

test('audit validates grant numbers instead of silently showing all events', async () => {
	await page.goto('/audit');
	const filter = page.getByRole('searchbox', { name: 'Grant number' });
	await filter.fill('#7');
	await expect(page).toHaveURL(/grant_id=7/);
	await filter.fill('not-a-number');
	await expect(page.getByText('Enter a positive grant number, such as 7 or #7.')).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Enter a valid grant number' })).toBeVisible();
	await expect(filter).toHaveAttribute('aria-invalid', 'true');
	await page.screenshot({ path: join(shots, 'audit-invalid-filter-desktop.png') });
	await page.getByRole('button', { name: 'Clear grant filter', exact: true }).click();
	await expect(page.getByTestId('audit-log')).toBeVisible();
});

async function olderAuditPages(count: number) {
	for (let i = 0; i < count; i++) {
		await page.getByRole('navigation', { name: 'Audit pages', exact: true }).getByRole('button', { name: 'Older events', exact: true }).click();
	}
}

async function auditFooterIsUnobscured() {
	const footer = page.getByRole('navigation', { name: 'Audit pages footer', exact: true });
	await footer.scrollIntoViewIfNeeded();
	for (const control of await footer.locator('button, a').all()) {
		const box = await control.boundingBox();
		expect(box!.height).toBeGreaterThanOrEqual(44);
		await expect(control).toBeInViewport();
		if (await control.isDisabled()) continue;
		expect(await control.evaluate((el) => {
			const r = el.getBoundingClientRect();
			return [0.15, 0.5, 0.85].every((x) => el.contains(document.elementFromPoint(r.left + r.width * x, r.top + r.height / 2)));
		})).toBe(true);
	}
}

test('older audit responses are discarded when their filter changes', async () => {
	const seed = await (await page.request.get('/api/admin/v1/audit')).json();
	const template = seed.events[0];
	const initial = Array.from({ length: 100 }, (_, index) => ({ ...template, id: 1000 - index, action: 'grant.requested' }));
	let releaseOlder!: () => void;
	let startedOlder!: () => void;
	const olderReady = new Promise<void>((resolve) => { startedOlder = resolve; });
	const olderRelease = new Promise<void>((resolve) => { releaseOlder = resolve; });
	await page.route('**/api/admin/v1/audit*', async (route) => {
		const params = new URL(route.request().url()).searchParams;
		if (params.has('before')) {
			startedOlder();
			await olderRelease;
			await route.fulfill({ json: { events: [{ ...template, id: 800, action: 'agent.disabled' }] } });
		} else if (params.get('action') === 'host.') {
			await route.fulfill({ json: { events: [{ ...template, id: 1100, action: 'host.created' }] } });
		} else {
			await route.fulfill({ json: { events: initial } });
		}
	});
	await page.goto('/audit');
	await olderAuditPages(4);
	await olderReady;
	await page.getByRole('radio', { name: 'Hosts', exact: true }).click();
	await expect(page.locator('[data-action="host.created"]')).toHaveCount(1);
	const response = page.waitForResponse((r) => new URL(r.url()).searchParams.has('before'));
	releaseOlder();
	await (await response).finished();
	await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
	await expect(page.locator('[data-action="agent.disabled"]')).toHaveCount(0);
	await expect(page.locator('[data-action="host.created"]')).toHaveCount(1);
	await page.unroute('**/api/admin/v1/audit*');
});

test('audit does not show empty or completion claims while filters update', async () => {
	const seed = await (await page.request.get('/api/admin/v1/audit')).json();
	const template = seed.events[0];
	let releaseAll!: () => void;
	let releaseHosts!: () => void;
	const all = new Promise<void>((resolve) => { releaseAll = resolve; });
	const hosts = new Promise<void>((resolve) => { releaseHosts = resolve; });
	await page.route('**/api/admin/v1/audit*', async (route) => {
		const params = new URL(route.request().url()).searchParams;
		if (params.has('grant_id')) {
			await route.fulfill({ json: { events: [] } });
		} else if (params.get('action') === 'host.') {
			await hosts;
			await route.fulfill({ json: { events: [{ ...template, id: 1200, action: 'host.created' }] } });
		} else {
			await all;
			await route.fulfill({ json: { events: [{ ...template, id: 1199, action: 'grant.requested' }] } });
		}
	});
	await page.goto('/audit?grant_id=99999');
	await expect(page.getByRole('heading', { name: 'No matching events' })).toBeVisible();
	await page.getByRole('searchbox', { name: 'Grant number' }).fill('');
	await expect(page.getByText(/Updating results/)).toBeVisible();
	await expect(page.getByRole('heading', { name: 'No matching events' })).toHaveCount(0);
	await expect(page.getByText('End of recorded history.')).toHaveCount(0);
	await page.screenshot({ path: join(shots, 'audit-updating-from-empty-desktop.png') });
	releaseAll();
	await expect(page.locator('[data-action="grant.requested"]')).toHaveCount(1);
	await page.getByRole('radio', { name: 'Hosts', exact: true }).click();
	await expect(page.getByText(/Updating results/)).toBeVisible();
	await expect(page.getByText('End of recorded history.')).toHaveCount(0);
	await page.screenshot({ path: join(shots, 'audit-updating-retained-desktop.png') });
	releaseHosts();
	await expect(page.locator('[data-action="host.created"]')).toHaveCount(1);
	await expect(page.getByText(/Updating results/)).toHaveCount(0);
	await expect(page.getByRole('navigation', { name: 'Audit pages', exact: true }).getByRole('button')).toHaveCount(0);
	await expect(page.getByRole('navigation', { name: 'Audit pages', exact: true })).toContainText('Event 1');
	await page.screenshot({ path: join(shots, 'audit-single-result-desktop.png') });
	await page.unroute('**/api/admin/v1/audit*');
});

for (const outcome of ['success', 'failure'] as const) {
	test(`Back to latest ignores an obsolete older-page ${outcome}`, async () => {
		const seed = await (await page.request.get('/api/admin/v1/audit')).json();
		const template = seed.events[0];
		const events = (first: number, action: string) => Array.from({ length: 100 }, (_, index) => ({ ...template, id: first - index, action }));
		let baseReads = 0;
		let resumedCursor = '';
		let release!: () => void;
		let started!: () => void;
		const pending = new Promise<void>((resolve) => { release = resolve; });
		const ready = new Promise<void>((resolve) => { started = resolve; });
		await page.route('**/api/admin/v1/audit*', async (route) => {
			const cursor = new URL(route.request().url()).searchParams.get('before');
			if (cursor === '801') {
				started();
				await pending;
				await route.fulfill(outcome === 'success'
					? { json: { events: events(800, 'agent.disabled') } }
					: { status: 500, json: { error: { code: 'test', message: 'Obsolete history request' } } });
			} else if (cursor === '901') {
				await route.fulfill({ json: { events: events(900, 'agent.created') } });
			} else if (cursor) {
				resumedCursor = cursor;
				await route.fulfill({ json: { events: events(1000, 'grant.extended') } });
			} else {
				baseReads++;
				await route.fulfill({ json: { events: baseReads === 1 ? events(1000, 'grant.requested') : events(1100, 'host.created') } });
			}
		});
		try {
			await page.goto('/audit');
			await olderAuditPages(4);
			await expect(page.locator('[data-action="agent.created"]')).toHaveCount(25);
			const viewport = outcome === 'success' ? { width: 320, height: 568 } : { width: 390, height: 844 };
			await page.setViewportSize(viewport);
			await olderAuditPages(4);
			await ready;
			await auditFooterIsUnobscured();
			await page.screenshot({ path: join(shots, `audit-pagination-loading-${viewport.width}.png`) });
			const footer = page.getByRole('navigation', { name: 'Audit pages footer', exact: true });
			await footer.getByRole('link', { name: 'Back to filters', exact: true }).click();
			await expect(page.getByRole('group', { name: 'Filters', exact: true })).toBeFocused();
			const controls = page.getByRole('navigation', { name: 'Audit pages', exact: true });
			await expect(controls.getByText('Live updates paused', { exact: true })).toBeVisible();
			await controls.scrollIntoViewIfNeeded();
			await page.screenshot({ path: join(shots, `audit-history-paused-${viewport.width}.png`) });
			// Exercise the footer reset: it must recover both position and focus.
			await footer.getByRole('button', { name: 'Back to latest', exact: true }).click();
			await expect(page.locator('[data-action="host.created"]')).toHaveCount(25);
			await expect(controls).toBeFocused();
			await expect(controls).toBeInViewport();
			await expect(controls.getByText('Events 1–25')).toBeVisible();
			const response = page.waitForResponse((r) => new URL(r.url()).searchParams.get('before') === '801');
			release();
			await (await response).finished();
			await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
			await expect(page.locator('[data-action="agent.disabled"], [data-action="agent.created"]')).toHaveCount(0);
			await expect(page.getByText(/Obsolete history request/)).toHaveCount(0);
			await expect(controls.getByText('Live updates paused', { exact: true })).toHaveCount(0);
			await olderAuditPages(4);
			await expect(page.locator('[data-action="grant.extended"]')).toHaveCount(25);
			expect(resumedCursor).toBe('1001');
		} finally {
			release();
			await page.unroute('**/api/admin/v1/audit*');
			await page.setViewportSize({ width: 1280, height: 800 });
		}
	});
}


test('phone audit pages stay bounded, navigate cached history and retry without skipping', async () => {
	const seed = await (await page.request.get('/api/admin/v1/audit')).json();
	const template = seed.events[0];
	for (const viewport of [{ width: 320, height: 568 }, { width: 390, height: 844 }]) {
		let reads = 0;
		let attempts = 0;
		const cursors: string[] = [];
		await page.route('**/api/admin/v1/audit*', async (route) => {
			const cursor = new URL(route.request().url()).searchParams.get('before');
			if (cursor) {
				cursors.push(cursor);
				attempts++;
				await route.fulfill(attempts === 1
					? { status: 500, json: { error: { code: 'test', message: 'Temporary history failure' } } }
					: { json: { events: Array.from({ length: 3 }, (_, i) => ({ ...template, id: 900 - i, action: 'agent.disabled' })) } });
			} else {
				reads++;
				await route.fulfill({ json: { events: Array.from({ length: 100 }, (_, i) => ({ ...template, id: 1000 - i, action: 'grant.requested' })) } });
			}
		});
		try {
			await page.setViewportSize(viewport);
			await page.goto('/audit');
			const top = page.getByRole('navigation', { name: 'Audit pages', exact: true });
			const footer = page.getByRole('navigation', { name: 'Audit pages footer', exact: true });
			await expect(page.locator('[data-action]')).toHaveCount(25);
			await expect(top.getByRole('button', { name: 'Newer events', exact: true })).toBeDisabled();
			await olderAuditPages(3);
			await expect(top.getByText('Events 76–100')).toBeVisible();
			await expect(top).toBeFocused();
			await top.getByRole('button', { name: 'Newer events', exact: true }).click();
			await expect(top.getByText('Events 51–75')).toBeVisible();
			expect(reads).toBe(1);
			expect(cursors).toEqual([]);
			await olderAuditPages(2);
			await expect(top.getByText(/Temporary history failure/)).toBeVisible();
			await expect(top.getByText('Events 76–100')).toBeVisible();
			await expect(page.locator('[data-action]')).toHaveCount(25);
			await auditFooterIsUnobscured();
			await page.screenshot({ path: join(shots, `audit-pagination-error-${viewport.width}.png`) });
			await footer.getByRole('button', { name: 'Older events', exact: true }).focus();
			await page.keyboard.press('Enter');
			await expect(top.getByText('Events 101–103')).toBeVisible();
			await expect(top).toBeFocused();
			await expect(page.locator('[data-action="agent.disabled"]')).toHaveCount(3);
			await expect(top.getByRole('button', { name: 'Older events', exact: true })).toBeDisabled();
			expect(cursors).toEqual(['901', '901']);
			await page.screenshot({ path: join(shots, `audit-pagination-history-${viewport.width}.png`) });
			await auditFooterIsUnobscured();
			await page.screenshot({ path: join(shots, `audit-pagination-end-${viewport.width}.png`) });
			await footer.getByRole('button', { name: 'Newer events', exact: true }).click();
			await expect(top.getByText('Events 76–100')).toBeVisible();
			await expect(top).toBeFocused();
			await expect(page.locator('[data-action="grant.requested"]')).toHaveCount(25);
			await expect(page.locator('[data-action="agent.disabled"]')).toHaveCount(0);
			expect(reads).toBe(1);
			for (let i = 0; i < 3; i++) await top.getByRole('button', { name: 'Newer events', exact: true }).click();
			await expect(top.getByText('Events 1–25')).toBeVisible();
			await expect(top.getByText('Live updates paused', { exact: true })).toBeVisible();
			expect(reads).toBe(1); // Browsing a snapshot never silently refreshes one part of it.
			await page.screenshot({ path: join(shots, `audit-pagination-snapshot-${viewport.width}.png`) });
			await top.getByRole('button', { name: 'Back to latest', exact: true }).click();
			await expect(top.getByText('Live updates paused', { exact: true })).toHaveCount(0);
			await expect.poll(() => reads).toBe(2);
			expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
		} finally {
			await page.unroute('**/api/admin/v1/audit*');
		}
	}
	await page.setViewportSize({ width: 1280, height: 800 });
});


test('audit history survives reconnects and empty boundaries preserve focus', async () => {
	const seed = await (await page.request.get('/api/admin/v1/audit')).json();
	const template = { ...seed.events[0], grant_id: 42 };
	const events = (first: number, action: string) => Array.from({ length: 100 }, (_, i) => ({ ...template, id: first - i, action }));
	const visibleIds = () => page.locator('[data-action] [aria-controls^="audit-"]').evaluateAll(els => els.map(el => Number(el.getAttribute('aria-controls')!.slice(6))));
	for (const viewport of [{ width: 320, height: 568 }, { width: 390, height: 844 }]) {
		let reads = 0;
		let advanced = false;
		const cursors: string[] = [];
		await page.route('**/api/admin/v1/audit*', async (route) => {
			const cursor = new URL(route.request().url()).searchParams.get('before');
			if (cursor) {
				cursors.push(cursor);
				await route.fulfill({ json: { events: cursor === '801' ? [] : events(Number(cursor) - 1, 'agent.created') } });
			} else {
				reads++;
				await route.fulfill({ json: { events: events(advanced ? 1001 : 1000, 'grant.requested') } });
			}
		});
		try {
			await page.setViewportSize(viewport);
			await page.goto('/audit');
			const top = page.getByRole('navigation', { name: 'Audit pages', exact: true });
			const footer = page.getByRole('navigation', { name: 'Audit pages footer', exact: true });
			await olderAuditPages(4); // Load a second server buffer: 1000..901, 900..801.
			await expect(page.locator('[data-action="agent.created"]')).toHaveCount(25);
			await top.getByRole('button', { name: 'Newer events', exact: true }).click();
			await expect(top.getByText('Events 76–100')).toBeVisible();
			expect(await visibleIds()).toEqual(Array.from({ length: 25 }, (_, i) => 925 - i));
			advanced = true; // A new latest event exists, but history must remain unchanged.
			const hostsRefreshed = page.waitForResponse(r => new URL(r.url()).pathname === '/api/admin/v1/hosts');
			await page.evaluate(() => window.dispatchEvent(new Event('offline')));
			await page.evaluate(() => window.dispatchEvent(new Event('online')));
			await (await hostsRefreshed).finished(); // Another query proves reconnect handling actually ran.
			await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
			expect(reads).toBe(1);
			expect(await visibleIds()).toEqual(Array.from({ length: 25 }, (_, i) => 925 - i));
			await expect(top.getByText('Live updates paused', { exact: true })).toBeVisible();
			await page.screenshot({ path: join(shots, `audit-reconnect-frozen-${viewport.width}.png`) });
			await olderAuditPages(1);
			expect(await visibleIds()).toEqual(Array.from({ length: 25 }, (_, i) => 900 - i)); // No 901/900 gap.
			await olderAuditPages(3);
			await expect(top.getByText('Events 176–200')).toBeVisible();
			await footer.getByRole('button', { name: 'Older events', exact: true }).focus();
			await page.keyboard.press('Enter'); // Exactly 200 events: the next response is empty.
			await expect(top.getByRole('button', { name: 'Older events', exact: true })).toBeDisabled();
			await expect(top).toBeFocused();
			await expect(top.locator('[aria-live="polite"]')).toContainText('End of recorded history.');
			await expect(top.getByText('Events 176–200')).toBeVisible();
			expect(await visibleIds()).toEqual(Array.from({ length: 25 }, (_, i) => 825 - i));
			expect(cursors).toEqual(['901', '801']);
			await page.screenshot({ path: join(shots, `audit-empty-boundary-${viewport.width}.png`) });
			await footer.getByRole('button', { name: 'Back to latest', exact: true }).click();
			await expect(top.getByText('Live updates paused', { exact: true })).toHaveCount(0);
			await expect.poll(() => reads).toBe(2);
			await expect.poll(visibleIds).toEqual(Array.from({ length: 25 }, (_, i) => 1001 - i));
			await olderAuditPages(4);
			await expect.poll(visibleIds).toEqual(Array.from({ length: 25 }, (_, i) => 901 - i));
			expect(cursors).toEqual(['901', '801', '902']); // Fresh pagination starts at the refreshed head.
		} finally {
			await page.unroute('**/api/admin/v1/audit*');
		}
	}
	await page.setViewportSize({ width: 1280, height: 800 });
});
