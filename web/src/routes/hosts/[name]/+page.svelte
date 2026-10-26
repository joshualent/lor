<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatCountdown, formatTimestamp } from '#lib/format.js';
	import { keys, fetchHost } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Row from '#lib/components/lor/Row.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import Warnings from '#lib/components/lor/Warnings.svelte';

	const name = $derived(page.params.name ?? '');
	const qc = useQueryClient();
	// Bootstrap links expire after 15 minutes: refetch well before that.
	const q = createQuery(() => ({
		queryKey: keys.host(name),
		queryFn: () => fetchHost(name),
		refetchInterval: 5 * 60_000
	}));
	const h = $derived(q.data?.host);

	let editing = $state(false);
	let form = $state({
		name: '',
		address: '',
		max_level: 'sudo' as Level,
		hypervisor: false,
		lor_infra: false,
		notes: '',
		disabled: false
	});
	function startEdit() {
		if (!h) return;
		form = {
			name: h.name,
			address: h.address,
			max_level: h.max_level,
			hypervisor: h.hypervisor,
			lor_infra: h.lor_infra,
			notes: h.notes,
			disabled: h.disabled
		};
		editing = true;
	}

	let busy = $state(false);
	let scan = $state<Schemas['ScanResult'] | null>(null);

	function refresh() {
		qc.invalidateQueries({ queryKey: keys.hosts });
	}

	async function save() {
		busy = true;
		try {
			const res = await call(
				api.PATCH('/hosts/{name}', { params: { path: { name } }, body: form })
			);
			editing = false;
			refresh();
			if (res.host.name !== name) {
				// The name is the certificate principal: the host's principals file
				// still has the old one until the bootstrap runs again.
				toast.success(`Renamed to ${res.host.name}. Re-run the bootstrap one-liner on the host.`, {
					duration: 10_000
				});
				await goto(`/hosts/${res.host.name}`, { replaceState: true });
			} else {
				toast.success('Saved');
			}
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function rescan() {
		busy = true;
		try {
			scan = await call(api.POST('/hosts/{name}/scan', { params: { path: { name } }, body: {} }));
			if (!scan.changed) toast.success(`Host key unchanged: ${scan.fingerprint}`);
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function acceptKey() {
		if (!h || !scan) return;
		busy = true;
		try {
			await stepUp({ kind: 'accept_host_key', host_id: h.id, host_key: scan.host_key });
			toast.success('New host key accepted');
			scan = null;
			refresh();
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}
</script>

{#if q.isPending}
	<p class="text-muted-foreground">Loading…</p>
{:else if q.isError}
	<p class="text-destructive">{errorMessage(q.error)}</p>
{:else if h && q.data}
	<PageHeader title={h.name} subtitle={h.address}>
		{#snippet actions()}
			{#if !h.disabled && h.host_key}<Button href="/grants/new?host_id={h.id}">Grant access</Button
				>{/if}
		{/snippet}
	</PageHeader>

	<div class="space-y-6">
		{#if q.data.scan_error}<p class="text-warning">
				Host key scan failed: {q.data.scan_error}
			</p>{/if}

		<Section title="Settings">
			{#snippet actions()}{#if !editing}<Button size="sm" variant="ghost" onclick={startEdit}
						>Edit</Button
					>{/if}{/snippet}
			{#if editing}
				<form
					class="space-y-4"
					onsubmit={(e) => {
						e.preventDefault();
						save();
					}}
				>
					<div class="space-y-2">
						<Label for="addr">LAN IP</Label><Input id="addr" bind:value={form.address} required />
					</div>
					<div class="space-y-2">
						<Label for="host-name">Name</Label><Input
							id="host-name"
							bind:value={form.name}
							autocapitalize="off"
							autocomplete="off"
							required
						/>
						<p class="text-sm text-muted-foreground">
							The certificate principal. Renaming needs no open grants and a fresh bootstrap run.
						</p>
					</div>
					<fieldset class="space-y-2">
						<legend class="mb-1 text-sm font-medium">Most access allowed</legend>
						<label class="flex items-center gap-2"
							><input
								type="radio"
								bind:group={form.max_level}
								value="sudo"
								class="size-4 accent-primary"
							/> sudo</label
						>
						<label class="flex items-center gap-2"
							><input
								type="radio"
								bind:group={form.max_level}
								value="user"
								class="size-4 accent-primary"
							/> standard user only</label
						>
					</fieldset>
					<label class="flex items-start gap-3"
						><Checkbox bind:checked={form.hypervisor} class="mt-0.5" /><span>Hypervisor</span
						></label
					>
					<label class="flex items-start gap-3"
						><Checkbox bind:checked={form.lor_infra} class="mt-0.5" /><span>Lor infrastructure</span
						></label
					>
					<label class="flex items-start gap-3"
						><Checkbox bind:checked={form.disabled} class="mt-0.5" /><span
							>Disabled (no new grants)</span
						></label
					>
					<div class="space-y-2">
						<Label for="notes">Notes</Label><Textarea id="notes" bind:value={form.notes} rows={2} />
					</div>
					<div class="flex gap-2">
						<Button type="submit" disabled={busy}>Save</Button><Button
							variant="ghost"
							onclick={() => (editing = false)}>Cancel</Button
						>
					</div>
				</form>
			{:else}
				<dl>
					<Row label="Most access">{h.max_level === 'sudo' ? 'sudo' : 'standard user only'}</Row>
					<Row label="Hypervisor">{h.hypervisor ? 'yes' : 'no'}</Row>
					<Row label="Lor infra">{h.lor_infra ? 'yes (grants default to 15 min)' : 'no'}</Row>
					<Row label="Status">{h.disabled ? 'disabled' : 'enabled'}</Row>
					<Row label="SSH server">{h.ssh_version || 'unknown (rescan the key)'}</Row>
					<Row label="Idle timeout"
						>{h.idle_timeout === 'enforced'
							? 'idle sessions close after 15 min'
							: h.idle_timeout === 'unenforced'
								? 'not enforced by this OpenSSH (needs 10.6+); sessions stay open until closed'
								: 'unknown'}</Row
					>
					{#if h.notes}<Row label="Notes"><span class="whitespace-pre-wrap">{h.notes}</span></Row
						>{/if}
				</dl>
			{/if}
		</Section>

		<Warnings warnings={h.warnings} level={h.max_level} />

		<Section title="Host key">
			{#if h.host_key_fingerprint}
				<p class="font-mono text-sm break-all">{h.host_key_fingerprint}</p>
				{#if h.host_key_seen_at}<p class="text-sm text-muted-foreground">
						Scanned {formatTimestamp(Date.parse(h.host_key_seen_at))}
					</p>{/if}
			{:else}
				<p class="text-muted-foreground">No host key yet. Grants need one.</p>
			{/if}
			<p class="mt-2 text-sm text-muted-foreground">
				Compare it with the fingerprint the bootstrap script prints.
			</p>
			<Button variant="secondary" class="mt-3" disabled={busy} onclick={rescan}>Rescan key</Button>
			{#if scan?.changed}
				<div
					class="mt-4 rounded-lg border border-warning/50 bg-warning/8 p-3"
					data-testid="key-changed"
				>
					<p class="font-medium text-warning">The host key changed</p>
					<p class="mt-1 text-sm">Was <code class="break-all">{scan.previous_fingerprint}</code></p>
					<p class="text-sm">Now <code class="break-all">{scan.fingerprint}</code></p>
					<p class="mt-2 text-sm text-muted-foreground">
						Only accept it if you rebuilt or re-keyed this host. Otherwise something is
						impersonating it.
					</p>
					<Button class="mt-3" disabled={busy} onclick={acceptKey}
						><KeyRound /> Accept new key</Button
					>
				</div>
			{/if}
		</Section>

		<Section title="Bootstrap">
			<p class="mb-3 text-sm text-muted-foreground">
				Run from your machine. The link works for {formatCountdown(
					Date.parse(q.data.bootstrap.expires_at),
					clock.now
				)}; reload for a fresh one.
			</p>
			<div class="space-y-4">
				<div class="space-y-1.5">
					<p class="text-sm">Install{h.max_level === 'sudo' ? '' : ' (no sudo user)'}</p>
					<CopyField value={q.data.bootstrap.install} />
				</div>
				{#if h.max_level === 'sudo'}
					<div class="space-y-1.5">
						<p class="text-sm">Install without <code>agent-sudo</code></p>
						<CopyField value={q.data.bootstrap.install_no_sudo} />
					</div>
				{/if}
				<div class="space-y-1.5">
					<p class="text-sm">Uninstall</p>
					<CopyField value={q.data.bootstrap.uninstall} />
				</div>
			</div>
		</Section>
	</div>
{/if}
