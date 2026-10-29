<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Bot from '@lucide/svelte/icons/bot';
	import Network from '@lucide/svelte/icons/network';
	import RotateCcwKey from '@lucide/svelte/icons/rotate-ccw-key';
	import Ban from '@lucide/svelte/icons/ban';
	import CirclePlay from '@lucide/svelte/icons/circle-play';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatCountdown, formatTimestamp } from '#lib/format.js';
	import { keys, fetchAgents, type Agent } from '#lib/queries.js';
	import { agentLook } from '#lib/status.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import Callout from '#lib/components/lor/Callout.svelte';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import DescriptionItem from '#lib/components/lor/DescriptionItem.svelte';
	import DescriptionList from '#lib/components/lor/DescriptionList.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListRow from '#lib/components/lor/ListRow.svelte';
	import ListSkeleton from '#lib/components/lor/ListSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';
	import SearchField from '#lib/components/lor/SearchField.svelte';

	const qc = useQueryClient();
	const agents = createQuery(() => ({ queryKey: keys.agents, queryFn: fetchAgents }));
	const refresh = () => qc.invalidateQueries({ queryKey: keys.agents });

	let search = $state('');
	const matching = $derived(
		(agents.data ?? []).filter(
			(a) =>
				!search.trim() ||
				`${a.name} ${a.lan_addrs.join(' ')}`.toLowerCase().includes(search.trim().toLowerCase())
		)
	);
	let addOpen = $state(false);
	let name = $state('');
	let ip = $state('');
	let busy = $state(false);
	let enrollment = $state<Schemas['Enrollment'] | null>(null);

	// The agent sheet (details + actions), and the dialogs it leads to.
	let selectedId = $state<number | null>(null);
	let sheetOpen = $state(false);
	const selected = $derived(agents.data?.find((a) => a.id === selectedId) ?? null);
	let editOpen = $state(false);
	let editIps = $state('');
	let rotateOpen = $state(false);
	let disableOpen = $state(false);

	const splitIps = (s: string) => s.split(/[\s,]+/).filter(Boolean);

	function openSheet(a: Agent) {
		selectedId = a.id;
		sheetOpen = true;
	}
	function fromSheet(next: () => void) {
		sheetOpen = false;
		next();
	}

	async function add() {
		busy = true;
		try {
			enrollment = await call(
				api.POST('/agents', { body: { name: name.trim(), lan_addrs: splitIps(ip) } })
			);
			addOpen = false;
			name = ip = '';
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function rotate() {
		if (!selected) return;
		busy = true;
		try {
			enrollment = await call(
				api.POST('/agents/{id}/rotate', { params: { path: { id: selected.id } }, body: {} })
			);
			rotateOpen = false;
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function saveIps() {
		if (!selected) return;
		busy = true;
		try {
			await stepUp({
				kind: 'set_agent_addrs',
				agent_id: selected.id,
				lan_addrs: splitIps(editIps)
			});
			toast.success(`Updated ${selected.name}'s LAN IPs`);
			editOpen = false;
			refresh();
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function setStatus(a: Agent, status: 'active' | 'disabled') {
		if (busy) return;
		busy = true;
		try {
			await call(
				api.POST('/agents/{id}/status', { params: { path: { id: a.id } }, body: { status } })
			);
			toast.success(
				status === 'disabled'
					? `${a.name} disabled; its open grants and offers were ended`
					: `${a.name} enabled`
			);
			sheetOpen = false;
			disableOpen = false;
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	const subtitle = $derived.by(() => {
		const all = agents.data;
		if (!all?.length) return undefined;
		const enrolled = all.filter((a) => a.status === 'active' && a.enrolled).length;
		const waiting = all.filter((a) => a.status === 'active' && !a.enrolled).length;
		const off = all.filter((a) => a.status === 'disabled').length;
		return [
			`${enrolled} enrolled`,
			waiting && `${waiting} waiting to enroll`,
			off && `${off} disabled`
		]
			.filter(Boolean)
			.join(' · ');
	});
</script>

<PageHeader title="Agents" subtitle="Manage the agents that can ask for access to your hosts.">
	{#snippet actions()}<Button onclick={() => (addOpen = true)}><Plus /> Add agent</Button>{/snippet}
</PageHeader>

<div class="space-y-7">
	{#if enrollment}
		<Callout tone="info" icon={KeyRound} title="Enroll {enrollment.agent.name}">
			Run this in the agent's LXC. It works once, for the next
			{formatCountdown(Date.parse(enrollment.expires_at), clock.now)}.
			<div class="mt-3"><CopyField value={enrollment.command} label="Copy enroll command" /></div>
			{#snippet action()}
				<Button variant="secondary" size="sm" onclick={() => (enrollment = null)}>Done</Button>
			{/snippet}
		</Callout>
	{/if}

	{#if agents.isError && agents.data}<StaleNotice
			error={agents.error}
			updatedAt={agents.dataUpdatedAt}
			fetching={agents.isFetching}
			onretry={() => agents.refetch()}
		/>{/if}
	{#if agents.isPending}
		<ListSkeleton rows={3} />
	{:else if agents.isError && !agents.data}
		<ErrorState
			error={agents.error}
			fetching={agents.isFetching}
			onretry={() => agents.refetch()}
			variant="inset"
		/>
	{:else if agents.data?.length === 0}
		<EmptyState
			icon={Bot}
			title="No agents yet"
			description="Add the agent's LXC by its fixed LAN IP, then run the enroll command there."
		>
			{#snippet action()}<Button variant="secondary" onclick={() => (addOpen = true)}
					><Plus /> Add agent</Button
				>{/snippet}
		</EmptyState>
	{:else if agents.data}
		<div class="flex flex-wrap items-center justify-between gap-3">
			<p class="text-callout text-muted-foreground">{subtitle}</p>
			<SearchField
				bind:value={search}
				label="Filter agents"
				placeholder="Find an agent by name or IP…"
				class="w-full sm:w-72"
			/>
		</div>
		{#if matching.length === 0}<EmptyState
				title="No matching agents"
				description="Try another name or address."
			/>{:else}
			<List label="Agents">
				{#each matching as a (a.id)}
					{@const look = agentLook(a)}
					<ListRow stackMeta onclick={() => openSheet(a)} chevron disabled={false}>
						{#snippet leading()}<IconTile text={a.name} tone={look.tone} />{/snippet}
						{#snippet titleContent()}
							<span class={a.status === 'disabled' ? 'text-muted-foreground' : ''}>{a.name}</span>
						{/snippet}
						{#snippet subtitleContent()}
							<span class="block font-mono text-footnote wrap-anywhere"
								>{a.lan_addrs.join(', ')}</span
							>
						{/snippet}
						{#snippet metaContent()}
							{#if look.badge}
								<Badge tone={look.badge.tone}>{look.badge.label}</Badge>
							{:else if a.last_seen_at}
								<span>seen {formatAgo(Date.parse(a.last_seen_at), clock.now)}</span>
							{/if}
						{/snippet}
					</ListRow>
				{/each}
			</List>
		{/if}
	{/if}
</div>

<!-- Agent sheet: details and actions -->
<Confirm bind:open={sheetOpen} title={selected?.name ?? 'Agent'}>
	{#if selected}
		{@const enrolled = selected.enrolled}
		<div class="space-y-4">
			<DescriptionList class="bg-background/60">
				<DescriptionItem label="Status"
					>{selected.status === 'disabled'
						? 'Disabled'
						: enrolled
							? 'Enrolled'
							: 'Not enrolled yet'}</DescriptionItem
				>
				<DescriptionItem label="LAN IPs" mono>{selected.lan_addrs.join(', ')}</DescriptionItem>
				{#if selected.last_seen_at}<DescriptionItem label="Last seen"
						>{formatAgo(Date.parse(selected.last_seen_at), clock.now)}</DescriptionItem
					>{/if}
				{#if selected.token_created_at}<DescriptionItem label="Token issued"
						>{formatTimestamp(Date.parse(selected.token_created_at))}</DescriptionItem
					>{/if}
				<DescriptionItem label="Added"
					>{formatTimestamp(Date.parse(selected.created_at))}</DescriptionItem
				>
			</DescriptionList>
			<List class="bg-background/60">
				<ListRow
					onclick={() =>
						fromSheet(() => {
							editIps = selected.lan_addrs.join(', ');
							editOpen = true;
						})}
					title="Edit LAN IPs"
					density="compact"
					chevron
				>
					{#snippet leading()}<IconTile icon={Network} size="sm" />{/snippet}
				</ListRow>
				<ListRow
					onclick={() => fromSheet(() => (rotateOpen = true))}
					title={enrolled ? 'Rotate token' : 'New enroll code'}
					density="compact"
					chevron
				>
					{#snippet leading()}<IconTile icon={RotateCcwKey} size="sm" />{/snippet}
				</ListRow>
				{#if selected.status === 'active'}
					<ListRow
						onclick={() => fromSheet(() => (disableOpen = true))}
						title="Disable"
						subtitle="Ends its open grants and offers"
						tone="danger"
						density="compact"
					>
						{#snippet leading()}<IconTile icon={Ban} tone="danger" size="sm" />{/snippet}
					</ListRow>
				{:else}
					<ListRow
						onclick={() => setStatus(selected, 'active')}
						title="Enable"
						tone="accent"
						density="compact"
					>
						{#snippet leading()}<IconTile icon={CirclePlay} tone="info" size="sm" />{/snippet}
					</ListRow>
				{/if}
			</List>
		</div>
	{/if}
	{#snippet footer()}
		<Button variant="outline" onclick={() => (sheetOpen = false)}>Close</Button>
	{/snippet}
</Confirm>

<Confirm
	bind:open={addOpen}
	title="Add agent"
	description="Lor accepts this agent's API calls and certificates only from these IPs."
>
	<form
		id="add-agent"
		class="space-y-5"
		onsubmit={(e) => {
			e.preventDefault();
			add();
		}}
	>
		<Field label="Name" for="a-name">
			<Input
				id="a-name"
				bind:value={name}
				placeholder="claude"
				autocapitalize="off"
				autocomplete="off"
				required
			/>
		</Field>
		<Field
			label="LAN IP"
			for="a-ip"
			hint="The agent LXC's fixed address. Separate several with commas."
		>
			<Input
				id="a-ip"
				bind:value={ip}
				placeholder="192.168.1.50"
				inputmode="decimal"
				autocomplete="off"
				required
			/>
		</Field>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (addOpen = false)}>Cancel</Button>
		<Button type="submit" form="add-agent" disabled={busy || !name || !ip}
			>{#if busy}<Spinner />{/if}Add agent</Button
		>
	{/snippet}
</Confirm>

<Confirm
	bind:open={editOpen}
	title="LAN IPs for {selected?.name}"
	description="New certificates are only usable from these addresses. Needs a passkey tap."
>
	<Field label="LAN IPs" for="e-ip" hint="Separate several with commas.">
		<Input id="e-ip" bind:value={editIps} inputmode="decimal" autocomplete="off" />
	</Field>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (editOpen = false)}>Cancel</Button>
		<Button disabled={busy || !editIps.trim()} onclick={saveIps}
			>{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : 'Save'}</Button
		>
	{/snippet}
</Confirm>

<Confirm
	bind:open={rotateOpen}
	title={selected?.enrolled
		? `Rotate ${selected?.name}'s token?`
		: `New enroll code for ${selected?.name}?`}
	description="The current token stops working right away. You'll get a new one-time enroll command."
>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (rotateOpen = false)}>Cancel</Button>
		<Button variant="destructive" disabled={busy} onclick={rotate}>Rotate</Button>
	{/snippet}
</Confirm>

<Confirm
	bind:open={disableOpen}
	title="Disable {selected?.name ?? 'agent'}?"
	description="This ends the agent's open grants and offers and denies its pending requests. Re-enabling it will not restore access."
>
	{#snippet footer()}
		<Button variant="outline" disabled={busy} onclick={() => (disableOpen = false)}>Cancel</Button>
		<Button
			variant="destructive"
			disabled={busy}
			onclick={() => selected && setStatus(selected, 'disabled')}
			>{#if busy}<Spinner />{/if}{busy ? 'Disabling…' : 'Disable agent'}</Button
		>
	{/snippet}
</Confirm>
