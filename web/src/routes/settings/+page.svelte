<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatTimestamp } from '#lib/format.js';
	import { keys, fetchPasskeys, fetchSessions } from '#lib/queries.js';
	import { addPasskey, stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';

	const qc = useQueryClient();
	const passkeys = createQuery(() => ({ queryKey: keys.passkeys, queryFn: fetchPasskeys }));
	const sessions = createQuery(() => ({ queryKey: keys.sessions, queryFn: fetchSessions }));

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
</script>

<PageHeader title="Settings" />

<div class="space-y-6">
	<Section title="Passkeys">
		{#snippet actions()}<Button size="sm" variant="secondary" onclick={() => (addOpen = true)}
				>Add passkey</Button
			>{/snippet}
		<ul class="-my-2 divide-y divide-border">
			{#each passkeys.data ?? [] as p (p.id)}
				<li class="flex items-center gap-3 py-3">
					<div class="min-w-0 flex-1">
						<p class="font-medium">{p.name}</p>
						<p class="text-sm text-muted-foreground">
							Added {formatTimestamp(Date.parse(p.created_at))}{p.last_used_at
								? ` · used ${formatAgo(Date.parse(p.last_used_at), clock.now)}`
								: ''}
						</p>
					</div>
					<Button
						size="sm"
						variant="ghost"
						onclick={() => {
							renameFor = p;
							renameTo = p.name;
							renameOpen = true;
						}}>Rename</Button
					>
					<Button
						size="sm"
						variant="ghost"
						class="text-destructive"
						disabled={(passkeys.data?.length ?? 0) <= 1}
						onclick={() => {
							removeFor = p;
							removeOpen = true;
						}}>Remove</Button
					>
				</li>
			{/each}
		</ul>
		<p class="mt-3 text-sm text-muted-foreground">
			Lost every passkey? Run <code>lor-server admin reset</code> on the Lor LXC console.
		</p>
	</Section>

	<Section title="Sessions">
		<ul class="-my-2 divide-y divide-border">
			{#each sessions.data ?? [] as s (s.id)}
				<li class="py-3 text-sm">
					<p class="break-words">
						{s.user_agent || 'Unknown browser'}{#if s.current}
							<span class="text-success">· this browser</span>{/if}
					</p>
					<p class="text-muted-foreground">
						{s.passkey_name ?? ''} · active {formatAgo(
							Date.parse(s.last_seen_at),
							clock.now
						)}{s.forwarded_for ? ` · ${s.forwarded_for}` : ''}
					</p>
				</li>
			{/each}
		</ul>
		<Button variant="destructive" class="mt-4" onclick={() => (everywhereOpen = true)}
			>Sign out everywhere</Button
		>
	</Section>
</div>

<Confirm
	bind:open={addOpen}
	title="Add a passkey"
	description="Confirm with an existing passkey first, then register the new one."
>
	<div class="space-y-2">
		<Label for="pk-name">Name</Label><Input
			id="pk-name"
			bind:value={newName}
			placeholder="Laptop"
			maxlength={64}
		/>
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (addOpen = false)}>Cancel</Button>
		<Button disabled={busy || !newName.trim()} onclick={add}
			><KeyRound /> {busy ? 'Waiting for passkey…' : 'Continue'}</Button
		>
	{/snippet}
</Confirm>

<Confirm bind:open={renameOpen} title="Rename passkey">
	<div class="space-y-2">
		<Label for="pk-rename">Name</Label><Input id="pk-rename" bind:value={renameTo} maxlength={64} />
	</div>
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
		<Button variant="destructive" disabled={busy} onclick={remove}><KeyRound /> Remove</Button>
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
