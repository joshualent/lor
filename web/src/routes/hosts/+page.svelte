<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import Server from '@lucide/svelte/icons/server';
	import Layers from '@lucide/svelte/icons/layers';
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level } from '#lib/api/client.js';
	import { errorMessage } from '#lib/errors.js';
	import { keys, fetchHosts, type Host } from '#lib/queries.js';
	import { hostLook } from '#lib/status.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListRow from '#lib/components/lor/ListRow.svelte';
	import ListSkeleton from '#lib/components/lor/ListSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import SearchField from '#lib/components/lor/SearchField.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';
	import SegmentedControl from '#lib/components/lor/SegmentedControl.svelte';
	import SettingsRow from '#lib/components/lor/SettingsRow.svelte';

	const qc = useQueryClient();
	const hosts = createQuery(() => ({ queryKey: keys.hosts, queryFn: fetchHosts }));

	let open = $state(false);
	let name = $state('');
	let address = $state('');
	let maxLevel = $state<Level>('sudo');
	let hypervisor = $state(false);
	let lorInfra = $state(false);
	let busy = $state(false);
	let search = $state('');

	async function add() {
		busy = true;
		try {
			const res = await call(
				api.POST('/hosts', {
					body: {
						name: name.trim(),
						address: address.trim(),
						max_level: maxLevel,
						hypervisor,
						lor_infra: lorInfra
					}
				})
			);
			if (res.scan_error)
				toast.warning(`Added ${res.host.name}, but the host key scan failed: ${res.scan_error}`);
			else toast.success(`Added ${res.host.name}`);
			qc.invalidateQueries({ queryKey: keys.hosts });
			open = false;
			await goto(`/hosts/${res.host.name}`);
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	const all = $derived(hosts.data ?? []);
	const matches = (h: Host) => {
		const s = search.trim().toLowerCase();
		return !s || h.name.toLowerCase().includes(s) || h.address.includes(s);
	};
	const enabled = $derived(all.filter((h) => !h.disabled && matches(h)));
	const disabled = $derived(all.filter((h) => h.disabled && matches(h)));
	const inventorySummary = $derived(
		hosts.data
			? `${all.length} ${all.length === 1 ? 'host' : 'hosts'}${all.some((h) => h.disabled) ? ` · ${all.filter((h) => h.disabled).length} disabled` : ''}`
			: undefined
	);
</script>

<PageHeader title="Hosts" subtitle="Manage the machines agents can request access to.">
	{#snippet actions()}<Button onclick={() => (open = true)}><Plus /> Add host</Button>{/snippet}
</PageHeader>

{#snippet hostRows(list: Host[])}
	<List label="Hosts">
		{#each list as h (h.id)}
			{@const look = hostLook(h, true)}
			<ListRow stackMeta href="/hosts/{h.name}">
				{#snippet leading()}<IconTile icon={look.icon} tone={look.tone} />{/snippet}
				{#snippet titleContent()}{h.name}{/snippet}
				{#snippet subtitleContent()}
					<span class="block font-mono text-footnote">{h.address}</span>
					<span class="block text-footnote"
						>{h.max_level === 'sudo' ? 'Up to sudo' : 'Standard user only'}</span
					>
				{/snippet}
				{#snippet metaContent()}
					{#if look.badges.length}
						<span class="flex flex-wrap justify-end gap-1">
							{#each look.badges as b (b.label)}<Badge tone={b.tone}>{b.label}</Badge>{/each}
						</span>
					{/if}
				{/snippet}
			</ListRow>
		{/each}
	</List>
{/snippet}

{#if hosts.isError && hosts.data}<StaleNotice
		error={hosts.error}
		updatedAt={hosts.dataUpdatedAt}
		fetching={hosts.isFetching}
		onretry={() => hosts.refetch()}
	/>{/if}

{#if hosts.isPending}
	<ListSkeleton rows={4} />
{:else if hosts.isError && !hosts.data}
	<ErrorState
		error={hosts.error}
		fetching={hosts.isFetching}
		onretry={() => hosts.refetch()}
		variant="inset"
	/>
{:else if all.length === 0}
	<EmptyState
		icon={Server}
		title="No hosts yet"
		description="Add a host by its LAN IP, then run its bootstrap one-liner so agents can log in."
	>
		{#snippet action()}<Button variant="secondary" onclick={() => (open = true)}
				><Plus /> Add host</Button
			>{/snippet}
	</EmptyState>
{:else}
	<div class="space-y-7">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<p class="text-callout text-muted-foreground">{inventorySummary}</p>
			<SearchField
				bind:value={search}
				label="Filter hosts"
				placeholder="Find a host by name or IP…"
				class="w-full sm:w-72"
			/>
		</div>
		{#if enabled.length === 0 && disabled.length === 0}
			<EmptyState title="No matching hosts" description="Try another name or address." />
		{/if}
		{#if enabled.length}
			<Section title={disabled.length ? 'Enabled' : undefined}>{@render hostRows(enabled)}</Section>
		{/if}
		{#if disabled.length}
			<Section
				title="Disabled"
				footnote="Disabled hosts can't be requested or offered. Re-enable one from its page."
			>
				{@render hostRows(disabled)}
			</Section>
		{/if}
	</div>
{/if}

<Confirm bind:open title="Add host" description="Lor scans its ed25519 host key right away.">
	<form
		class="space-y-5"
		id="add-host"
		onsubmit={(e) => {
			e.preventDefault();
			add();
		}}
	>
		<Field
			label="Name"
			for="h-name"
			hint="Lowercase letters, digits and dashes. It becomes the certificate principal."
		>
			<Input
				id="h-name"
				bind:value={name}
				placeholder="e.g. nas"
				autocapitalize="off"
				autocomplete="off"
				required
			/>
		</Field>
		<Field
			label="LAN IP"
			for="h-addr"
			hint="An IP, not a name, so traffic always stays on the LAN."
		>
			<Input
				id="h-addr"
				bind:value={address}
				placeholder="e.g. 192.168.1.20"
				inputmode="decimal"
				autocomplete="off"
				required
			/>
		</Field>
		<div class="space-y-2">
			<p class="text-sm font-medium" aria-hidden="true">Most access allowed</p>
			<SegmentedControl
				label="Most access allowed"
				fill
				bind:value={maxLevel}
				options={[
					{ value: 'sudo', label: 'sudo' },
					{ value: 'user', label: 'Standard user only' }
				]}
			/>
		</div>
		<ul
			aria-label="Host flags"
			class="divide-y divide-separator overflow-hidden rounded-xl bg-card"
		>
			<SettingsRow
				title="Hypervisor"
				description="A Proxmox host: sudo controls every guest."
				icon={Layers}
				tone="warning"
				for="h-hypervisor"
			>
				{#snippet control()}<Switch id="h-hypervisor" bind:checked={hypervisor} />{/snippet}
			</SettingsRow>
			<SettingsRow
				title="Lor infrastructure"
				description="Runs Lor, or the Caddy proxy in front of it."
				icon={ShieldAlert}
				tone="danger"
				for="h-infra"
			>
				{#snippet control()}<Switch id="h-infra" bind:checked={lorInfra} />{/snippet}
			</SettingsRow>
		</ul>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
		<Button type="submit" form="add-host" disabled={busy || !name || !address}
			>{#if busy}<Spinner />{/if}{busy ? 'Adding…' : 'Add host'}</Button
		>
	{/snippet}
</Confirm>
