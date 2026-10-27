<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Plus from '@lucide/svelte/icons/plus';
	import Pencil from '@lucide/svelte/icons/pencil';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Layers from '@lucide/svelte/icons/layers';
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import Ban from '@lucide/svelte/icons/ban';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatCountdown, formatTimestamp } from '#lib/format.js';
	import { keys, fetchHost } from '#lib/queries.js';
	import { hostLook } from '#lib/status.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import Callout from '#lib/components/lor/Callout.svelte';
	import Card from '#lib/components/lor/Card.svelte';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import DescriptionItem from '#lib/components/lor/DescriptionItem.svelte';
	import DescriptionList from '#lib/components/lor/DescriptionList.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import PageSkeleton from '#lib/components/lor/PageSkeleton.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import SegmentedControl from '#lib/components/lor/SegmentedControl.svelte';
	import SettingsRow from '#lib/components/lor/SettingsRow.svelte';
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

	let busy = $state<string | null>(null);
	let scan = $state<Schemas['ScanResult'] | null>(null);

	function refresh() {
		qc.invalidateQueries({ queryKey: keys.hosts });
	}

	async function save() {
		busy = 'save';
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
			busy = null;
		}
	}

	async function rescan() {
		busy = 'scan';
		try {
			scan = await call(api.POST('/hosts/{name}/scan', { params: { path: { name } }, body: {} }));
			if (!scan.changed) toast.success(`Host key unchanged: ${scan.fingerprint}`);
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = null;
		}
	}

	async function acceptKey() {
		if (!h || !scan) return;
		busy = 'accept';
		try {
			await stepUp({ kind: 'accept_host_key', host_id: h.id, host_key: scan.host_key });
			toast.success('New host key accepted');
			scan = null;
			refresh();
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = null;
		}
	}

	const look = $derived(h ? hostLook(h, true) : null);
	const yesNo = (b: boolean) => (b ? 'Yes' : 'No');
</script>

{#if q.isPending}
	<PageSkeleton />
{:else if q.isError}
	<ErrorState
		error={q.error}
		onretry={() => q.refetch()}
		back={{ href: '/hosts', label: 'All hosts' }}
	/>
{:else if h && q.data && look}
	<PageHeader title={h.name} back={{ href: '/hosts', label: 'Hosts' }}>
		{#snippet leading()}<IconTile icon={look.icon} tone={look.tone} size="lg" />{/snippet}
		{#snippet heading()}
			{h.name}
			<span
				class="mt-1 block font-mono text-callout font-normal tracking-normal text-muted-foreground"
				>{h.address}</span
			>
		{/snippet}
		{#snippet meta()}
			{#each look.badges as b (b.label)}<Badge tone={b.tone} size="md">{b.label}</Badge>{/each}
			<Badge size="md">{h.max_level === 'sudo' ? 'Up to sudo' : 'Standard user only'}</Badge>
		{/snippet}
		{#snippet actions()}
			{#if !h.disabled && h.host_key}<Button href="/grants/new?host_id={h.id}"
					><Plus /> Grant access</Button
				>{/if}
		{/snippet}
	</PageHeader>

	<div class="space-y-7">
		{#if q.data.scan_error}
			<Callout tone="warning" title="Host key scan failed">{q.data.scan_error}</Callout>
		{/if}

		{#if !h.host_key_fingerprint}
			<Callout tone="warning" title="No host key yet">
				Grants need one. Check that the host is reachable on <span class="font-mono"
					>{h.address}</span
				>, then rescan.
				{#snippet action()}
					<Button size="sm" disabled={busy !== null} onclick={rescan}
						>{#if busy === 'scan'}<Spinner />{:else}<RefreshCw />{/if} Rescan key</Button
					>
				{/snippet}
			</Callout>
		{/if}

		<Warnings warnings={h.warnings} level={h.max_level} />

		<Section title="Settings">
			{#snippet action()}
				{#if !editing}<Button size="sm" variant="ghost" onclick={startEdit}><Pencil /> Edit</Button
					>{/if}
			{/snippet}
			{#if editing}
				<form
					class="space-y-4"
					onsubmit={(e) => {
						e.preventDefault();
						save();
					}}
				>
					<Card class="space-y-5">
						<Field label="LAN IP" for="addr">
							<Input id="addr" bind:value={form.address} inputmode="decimal" required />
						</Field>
						<Field
							label="Name"
							for="host-name"
							hint="The certificate principal. Renaming needs no open grants and a fresh bootstrap run."
						>
							<Input
								id="host-name"
								bind:value={form.name}
								autocapitalize="off"
								autocomplete="off"
								required
							/>
						</Field>
						<div class="space-y-2">
							<p class="text-sm font-medium" aria-hidden="true">Most access allowed</p>
							<SegmentedControl
								label="Most access allowed"
								fill
								bind:value={form.max_level}
								options={[
									{ value: 'sudo', label: 'sudo' },
									{ value: 'user', label: 'Standard user only' }
								]}
							/>
						</div>
					</Card>
					<ul class="divide-y divide-separator overflow-hidden rounded-xl bg-card">
						<SettingsRow
							title="Hypervisor"
							description="A Proxmox host: sudo controls every guest."
							icon={Layers}
							tone="warning"
							for="e-hypervisor"
						>
							{#snippet control()}<Switch
									id="e-hypervisor"
									bind:checked={form.hypervisor}
								/>{/snippet}
						</SettingsRow>
						<SettingsRow
							title="Lor infrastructure"
							description="Runs Lor or its proxy. Grants default to 15 minutes."
							icon={ShieldAlert}
							tone="danger"
							for="e-infra"
						>
							{#snippet control()}<Switch id="e-infra" bind:checked={form.lor_infra} />{/snippet}
						</SettingsRow>
						<SettingsRow
							title="Disabled"
							description="No new grants; open ones end when you save."
							icon={Ban}
							for="e-disabled"
						>
							{#snippet control()}<Switch id="e-disabled" bind:checked={form.disabled} />{/snippet}
						</SettingsRow>
					</ul>
					<Card>
						<Field label="Notes" for="notes" optional>
							<Textarea id="notes" bind:value={form.notes} rows={2} />
						</Field>
					</Card>
					<div class="flex gap-2">
						<Button type="submit" disabled={busy !== null}
							>{#if busy === 'save'}<Spinner />{/if}Save</Button
						>
						<Button variant="ghost" onclick={() => (editing = false)}>Cancel</Button>
					</div>
				</form>
			{:else}
				<DescriptionList>
					<DescriptionItem label="Most access"
						>{h.max_level === 'sudo' ? 'sudo' : 'Standard user only'}</DescriptionItem
					>
					<DescriptionItem label="Hypervisor">{yesNo(h.hypervisor)}</DescriptionItem>
					<DescriptionItem label="Lor infrastructure"
						>{h.lor_infra ? 'Yes · grants default to 15 min' : 'No'}</DescriptionItem
					>
					<DescriptionItem label="Status">{h.disabled ? 'Disabled' : 'Enabled'}</DescriptionItem>
					<DescriptionItem label="SSH server"
						>{h.ssh_version || 'Unknown (rescan the key)'}</DescriptionItem
					>
					<DescriptionItem
						label="Idle timeout"
						layout={h.idle_timeout === 'unenforced' ? 'stacked' : 'inline'}
					>
						{#if h.idle_timeout === 'enforced'}
							Idle sessions close after 15 min
						{:else if h.idle_timeout === 'unenforced'}
							<span class="text-warning">Not enforced</span>
							<span class="text-callout text-muted-foreground"
								>· this OpenSSH needs 10.6+; sessions stay open until closed</span
							>
						{:else}
							Unknown
						{/if}
					</DescriptionItem>
					{#if h.notes}<DescriptionItem label="Notes" layout="stacked"
							><span class="whitespace-pre-wrap">{h.notes}</span></DescriptionItem
						>{/if}
				</DescriptionList>
			{/if}
		</Section>

		{#if h.host_key_fingerprint || scan?.changed}
			<Section
				title="Host key"
				footnote="Compare it with the fingerprint the bootstrap script prints."
			>
				{#snippet action()}
					<Button size="sm" variant="ghost" disabled={busy !== null} onclick={rescan}
						>{#if busy === 'scan'}<Spinner />{:else}<RefreshCw />{/if} Rescan</Button
					>
				{/snippet}
				<div class="space-y-3">
					{#if h.host_key_fingerprint}
						<DescriptionList>
							<DescriptionItem label="Fingerprint (ed25519)" layout="stacked" mono
								>{h.host_key_fingerprint}</DescriptionItem
							>
							{#if h.host_key_seen_at}
								<DescriptionItem label="Scanned">
									<span title={formatTimestamp(Date.parse(h.host_key_seen_at))}
										>{formatAgo(Date.parse(h.host_key_seen_at), clock.now)}</span
									>
								</DescriptionItem>
							{/if}
						</DescriptionList>
					{/if}
					{#if scan?.changed}
						<div data-testid="key-changed">
							<Callout tone="danger" title="The host key changed">
								<p>Was <code class="wrap-anywhere">{scan.previous_fingerprint}</code></p>
								<p>Now <code class="wrap-anywhere">{scan.fingerprint}</code></p>
								<p class="mt-2 text-muted-foreground">
									Only accept it if you rebuilt or re-keyed this host. Otherwise something is
									impersonating it.
								</p>
								{#snippet action()}
									<Button disabled={busy !== null} onclick={acceptKey}
										>{#if busy === 'accept'}<Spinner />{:else}<KeyRound />{/if} Accept new key</Button
									>
								{/snippet}
							</Callout>
						</div>
					{/if}
				</div>
			</Section>
		{/if}

		<Section
			title="Bootstrap"
			description="Run from your machine. The link works for {formatCountdown(
				Date.parse(q.data.bootstrap.expires_at),
				clock.now
			)}; reload for a fresh one."
		>
			<Card class="space-y-4">
				<CopyField
					caption="Install{h.max_level === 'sudo' ? '' : ' (no sudo user)'}"
					value={q.data.bootstrap.install}
					label="Copy install command"
				/>
				{#if h.max_level === 'sudo'}
					<CopyField
						caption="Install without agent-sudo"
						value={q.data.bootstrap.install_no_sudo}
						label="Copy install command without sudo"
					/>
				{/if}
				<CopyField
					caption="Uninstall"
					value={q.data.bootstrap.uninstall}
					label="Copy uninstall command"
				/>
			</Card>
		</Section>
	</div>
{/if}
