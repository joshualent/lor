<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Plus from '@lucide/svelte/icons/plus';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import Tablet from '@lucide/svelte/icons/tablet';
	import Laptop from '@lucide/svelte/icons/laptop';
	import Globe from '@lucide/svelte/icons/globe';
	import LogOut from '@lucide/svelte/icons/log-out';
	import ShieldOff from '@lucide/svelte/icons/shield-off';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatDuration, formatTimestamp } from '#lib/format.js';
	import { keys, fetchPasskeys, fetchSession, fetchSessions } from '#lib/queries.js';
	import { describeUserAgent } from '#lib/useragent.js';
	import { addPasskey, stepUp } from '#lib/webauthn.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import DescriptionItem from '#lib/components/lor/DescriptionItem.svelte';
	import DescriptionList from '#lib/components/lor/DescriptionList.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListRow from '#lib/components/lor/ListRow.svelte';
	import ListSkeleton from '#lib/components/lor/ListSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';

	const qc = useQueryClient();
	const passkeys = createQuery(() => ({ queryKey: keys.passkeys, queryFn: fetchPasskeys }));
	const sessions = createQuery(() => ({ queryKey: keys.sessions, queryFn: fetchSessions }));
	const session = createQuery(() => ({ queryKey: keys.session, queryFn: fetchSession }));

	let busy = $state(false);
	let addOpen = $state(false);
	let newName = $state('');
	let renameFor = $state<Schemas['Passkey'] | null>(null);
	let renameOpen = $state(false);
	let renameTo = $state('');
	let removeFor = $state<Schemas['Passkey'] | null>(null);
	let removeOpen = $state(false);
	let everywhereOpen = $state(false);

	async function act(fn: () => Promise<unknown>, ok: string) {
		busy = true;
		try {
			await fn();
			toast.success(ok);
			qc.invalidateQueries({ queryKey: keys.passkeys });
			qc.invalidateQueries({ queryKey: keys.sessions });
			return true;
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
			return false;
		} finally {
			busy = false;
		}
	}

	async function add() {
		if (await act(() => addPasskey(newName.trim()), `Added passkey ${newName.trim()}`)) {
			addOpen = false;
			newName = '';
		}
	}
	async function rename() {
		if (!renameFor) return;
		const id = renameFor.id;
		if (
			await act(
				() =>
					call(
						api.PATCH('/passkeys/{id}', {
							params: { path: { id } },
							body: { name: renameTo.trim() }
						})
					),
				'Renamed'
			)
		) {
			renameOpen = false;
		}
	}
	async function remove() {
		if (!removeFor) return;
		const id = removeFor.id;
		if (
			await act(
				() => stepUp({ kind: 'remove_passkey', passkey_id: id }),
				`Removed ${removeFor.name}`
			)
		) {
			removeOpen = false;
			// Removing the passkey this session used ends the session too.
			qc.invalidateQueries({ queryKey: keys.session });
		}
	}
	async function signOut() {
		await call(api.POST('/logout', { body: {} })).catch(() => {});
		qc.clear();
		await goto('/login', { replaceState: true });
	}
	async function signOutEverywhere() {
		await act(() => call(api.POST('/sessions/revoke-all', { body: {} })), 'Signed out everywhere');
		qc.clear();
		await goto('/login', { replaceState: true });
	}

	const DEVICE_ICONS = { phone: Smartphone, tablet: Tablet, desktop: Laptop, unknown: Globe };
	const onlyOne = $derived((passkeys.data?.length ?? 0) <= 1);
	const config = $derived(session.data?.config);
</script>

<PageHeader title="Settings" subtitle="Passkeys, sessions and how this Lor is set up." />

<div class="space-y-8">
	<Section
		title="Passkeys"
		count={passkeys.data?.length}
		footnote="Lost every passkey? Run lor-server admin reset on the Lor LXC console."
	>
		{#snippet action()}
			<Button size="sm" variant="ghost" onclick={() => (addOpen = true)}
				><Plus /> Add passkey</Button
			>
		{/snippet}
		{#if passkeys.isPending}
			<ListSkeleton rows={2} />
		{:else if passkeys.isError && !passkeys.data}
			<ErrorState error={passkeys.error} onretry={() => passkeys.refetch()} variant="inset" />
		{:else}
			<List>
				{#each passkeys.data ?? [] as p (p.id)}
					<ListRow
						subtitle="Added {formatTimestamp(Date.parse(p.created_at))}{p.last_used_at
							? ` · used ${formatAgo(Date.parse(p.last_used_at), clock.now)}`
							: ''}"
					>
						{#snippet leading()}<IconTile icon={KeyRound} tone="info" />{/snippet}
						{#snippet titleContent()}
							<span class="flex flex-wrap items-center gap-2">
								{p.name}
								{#if session.data?.passkey_name === p.name}<Badge tone="success">This session</Badge
									>{/if}
							</span>
						{/snippet}
						{#snippet trailing()}
							<Button
								size="icon-sm"
								variant="ghost"
								aria-label="Rename {p.name}"
								onclick={() => {
									renameFor = p;
									renameTo = p.name;
									renameOpen = true;
								}}><Pencil /></Button
							>
							<Button
								size="icon-sm"
								variant="ghost"
								class="text-destructive hover:text-destructive"
								aria-label="Remove {p.name}"
								title={onlyOne ? "The last passkey can't be removed" : undefined}
								disabled={onlyOne}
								onclick={() => {
									removeFor = p;
									removeOpen = true;
								}}><Trash2 /></Button
							>
						{/snippet}
					</ListRow>
				{/each}
			</List>
		{/if}
	</Section>

	<Section title="Sessions" count={sessions.data?.length}>
		{#if sessions.isPending}
			<ListSkeleton rows={2} />
		{:else if sessions.isError && !sessions.data}
			<ErrorState error={sessions.error} onretry={() => sessions.refetch()} variant="inset" />
		{:else}
			<List>
				{#each sessions.data ?? [] as s (s.id)}
					{@const device = describeUserAgent(s.user_agent)}
					<ListRow
						subtitle={[
							s.passkey_name,
							`active ${formatAgo(Date.parse(s.last_seen_at), clock.now)}`,
							s.forwarded_for
						]
							.filter(Boolean)
							.join(' · ')}
					>
						{#snippet leading()}<IconTile
								icon={DEVICE_ICONS[device.kind]}
								tone={s.current ? 'success' : 'neutral'}
							/>{/snippet}
						{#snippet titleContent()}
							<span class="flex flex-wrap items-center gap-2" title={s.user_agent}>
								{device.label}
								{#if s.current}<Badge tone="success">This browser</Badge>{/if}
							</span>
						{/snippet}
					</ListRow>
				{/each}
			</List>
		{/if}
	</Section>

	<Section>
		<List label="Sign out">
			<ListRow onclick={signOut} title="Sign out" density="compact">
				{#snippet leading()}<IconTile icon={LogOut} size="sm" />{/snippet}
			</ListRow>
			<ListRow
				onclick={() => (everywhereOpen = true)}
				title="Sign out everywhere"
				subtitle="Ends every session, including this one"
				tone="danger"
				density="compact"
			>
				{#snippet leading()}<IconTile icon={ShieldOff} tone="danger" size="sm" />{/snippet}
			</ListRow>
		</List>
	</Section>

	{#if config}
		<Section title="About this Lor" footnote="Set in lor-server.toml on the Lor LXC.">
			<DescriptionList>
				<DescriptionItem label="Public URL" mono>{config.public_url}</DescriptionItem>
				<DescriptionItem label="Default grant"
					>{formatDuration(config.default_grant_duration_s)}</DescriptionItem
				>
				<DescriptionItem label="Agent requests up to"
					>{formatDuration(config.agent_request_max_s)}</DescriptionItem
				>
				<DescriptionItem label="Longest grant"
					>{formatDuration(config.absolute_max_duration_s)}</DescriptionItem
				>
				<DescriptionItem label="Requests expire after"
					>{formatDuration(config.request_approval_timeout_s)}</DescriptionItem
				>
			</DescriptionList>
		</Section>
	{/if}
</div>

<Confirm
	bind:open={addOpen}
	title="Add a passkey"
	description="Confirm with an existing passkey first, then register the new one."
>
	<Field label="Name" for="pk-name" hint="So you can tell your passkeys apart.">
		<Input id="pk-name" bind:value={newName} placeholder="Laptop" maxlength={64} />
	</Field>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (addOpen = false)}>Cancel</Button>
		<Button disabled={busy || !newName.trim()} onclick={add}
			>{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : 'Continue'}</Button
		>
	{/snippet}
</Confirm>

<Confirm bind:open={renameOpen} title="Rename passkey">
	<Field label="Name" for="pk-rename">
		<Input id="pk-rename" bind:value={renameTo} maxlength={64} />
	</Field>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (renameOpen = false)}>Cancel</Button>
		<Button disabled={busy || !renameTo.trim()} onclick={rename}>Save</Button>
	{/snippet}
</Confirm>

<Confirm
	bind:open={removeOpen}
	title="Remove {removeFor?.name}?"
	description="It can no longer sign in, and its sessions end. Needs a passkey tap."
>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" disabled={busy} onclick={remove}
			>{#if busy}<Spinner />{:else}<KeyRound />{/if} Remove</Button
		>
	{/snippet}
</Confirm>

<Confirm
	bind:open={everywhereOpen}
	title="Sign out everywhere?"
	description="Every session ends, including this one."
>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (everywhereOpen = false)}>Cancel</Button>
		<Button variant="destructive" onclick={signOutEverywhere}>Sign out everywhere</Button>
	{/snippet}
</Confirm>
