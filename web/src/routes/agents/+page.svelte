<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatCountdown } from '#lib/format.js';
	import { keys, fetchAgents, type Agent } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';

	const qc = useQueryClient();
	const agents = createQuery(() => ({ queryKey: keys.agents, queryFn: fetchAgents }));
	const refresh = () => qc.invalidateQueries({ queryKey: keys.agents });

	let addOpen = $state(false);
	let name = $state('');
	let ip = $state('');
	let busy = $state(false);
	let enrollment = $state<Schemas['Enrollment'] | null>(null);

	let editFor = $state<Agent | null>(null);
	let editOpen = $state(false);
	let editIps = $state('');

	let rotateFor = $state<Agent | null>(null);
	let rotateOpen = $state(false);

	const splitIps = (s: string) => s.split(/[\s,]+/).filter(Boolean);

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
		if (!rotateFor) return;
		busy = true;
		try {
			enrollment = await call(
				api.POST('/agents/{id}/rotate', { params: { path: { id: rotateFor.id } }, body: {} })
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
		if (!editFor) return;
		busy = true;
		try {
			await stepUp({ kind: 'set_agent_addrs', agent_id: editFor.id, lan_addrs: splitIps(editIps) });
			toast.success(`Updated ${editFor.name}'s LAN IPs`);
			editOpen = false;
			refresh();
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	async function setStatus(a: Agent, status: 'active' | 'disabled') {
		try {
			await call(
				api.POST('/agents/{id}/status', { params: { path: { id: a.id } }, body: { status } })
			);
			toast.success(status === 'disabled' ? `${a.name} disabled` : `${a.name} enabled`);
			refresh();
		} catch (e) {
			toast.error(errorMessage(e));
		}
	}
</script>

<PageHeader title="Agents">
	{#snippet actions()}<Button onclick={() => (addOpen = true)}><Plus /> Add agent</Button>{/snippet}
</PageHeader>

{#if enrollment}
	<Section title="Enroll {enrollment.agent.name}" class="mb-6">
		<p class="mb-3">
			Run this in the agent's LXC. It works once, for the next {formatCountdown(
				Date.parse(enrollment.expires_at),
				clock.now
			)}.
		</p>
		<CopyField value={enrollment.command} label="Copy enroll command" />
		<Button variant="ghost" class="mt-3" onclick={() => (enrollment = null)}>Done</Button>
	</Section>
{/if}

<Section>
	{#if agents.isPending}
		<p class="text-muted-foreground">Loading…</p>
	{:else if agents.data?.length === 0}
		<p class="text-muted-foreground">No agents yet. Add one with its LXC's fixed LAN IP.</p>
	{:else if agents.data}
		<ul class="-my-2 divide-y divide-border">
			{#each agents.data as a (a.id)}
				<li class="py-3">
					<p>
						<span class="font-medium">{a.name}</span>
						<span class="font-mono text-sm text-muted-foreground">{a.lan_addrs.join(', ')}</span>
					</p>
					<p class="text-sm text-muted-foreground">
						{a.status === 'disabled'
							? 'Disabled'
							: a.enrolled
								? 'Enrolled'
								: 'Not enrolled yet'}{a.last_seen_at
							? ` · seen ${formatAgo(Date.parse(a.last_seen_at), clock.now)}`
							: ''}
					</p>
					<div class="mt-2 flex flex-wrap gap-1">
						<Button
							size="sm"
							variant="ghost"
							onclick={() => {
								editFor = a;
								editIps = a.lan_addrs.join(', ');
								editOpen = true;
							}}>Edit IPs</Button
						>
						<Button
							size="sm"
							variant="ghost"
							onclick={() => {
								rotateFor = a;
								rotateOpen = true;
							}}>{a.enrolled ? 'Rotate token' : 'New enroll code'}</Button
						>
						{#if a.status === 'active'}
							<Button
								size="sm"
								variant="ghost"
								class="text-destructive"
								onclick={() => setStatus(a, 'disabled')}>Disable</Button
							>
						{:else}
							<Button size="sm" variant="ghost" onclick={() => setStatus(a, 'active')}
								>Enable</Button
							>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</Section>

<Confirm
	bind:open={addOpen}
	title="Add agent"
	description="Lor accepts this agent's API calls and certificates only from these IPs."
>
	<form
		id="add-agent"
		class="space-y-4"
		onsubmit={(e) => {
			e.preventDefault();
			add();
		}}
	>
		<div class="space-y-2">
			<Label for="a-name">Name</Label><Input
				id="a-name"
				bind:value={name}
				placeholder="claude"
				autocapitalize="off"
				autocomplete="off"
				required
			/>
		</div>
		<div class="space-y-2">
			<Label for="a-ip">LAN IP</Label>
			<Input
				id="a-ip"
				bind:value={ip}
				placeholder="192.168.1.50"
				inputmode="decimal"
				autocomplete="off"
				required
			/>
			<p class="text-sm text-muted-foreground">
				The agent LXC's fixed address. Separate several with commas.
			</p>
		</div>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (addOpen = false)}>Cancel</Button>
		<Button type="submit" form="add-agent" disabled={busy || !name || !ip}>Add agent</Button>
	{/snippet}
</Confirm>

<Confirm
	bind:open={editOpen}
	title="LAN IPs for {editFor?.name}"
	description="New certificates are only usable from these addresses. Needs a passkey tap."
>
	<div class="space-y-2">
		<Label for="e-ip">LAN IPs</Label><Input id="e-ip" bind:value={editIps} />
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (editOpen = false)}>Cancel</Button>
		<Button disabled={busy || !editIps.trim()} onclick={saveIps}
			><KeyRound /> {busy ? 'Waiting for passkey…' : 'Save'}</Button
		>
	{/snippet}
</Confirm>

<Confirm
	bind:open={rotateOpen}
	title={rotateFor?.enrolled
		? `Rotate ${rotateFor?.name}'s token?`
		: `New enroll code for ${rotateFor?.name}?`}
	description="The current token stops working right away. You'll get a new one-time enroll command."
>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (rotateOpen = false)}>Cancel</Button>
		<Button variant="destructive" disabled={busy} onclick={rotate}>Rotate</Button>
	{/snippet}
</Confirm>
