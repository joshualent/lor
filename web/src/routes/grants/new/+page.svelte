<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Send from '@lucide/svelte/icons/send';
	import Bot from '@lucide/svelte/icons/bot';
	import Server from '@lucide/svelte/icons/server';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatClock, formatDuration, offerChatLine } from '#lib/format.js';
	import { keys, fetchAgents, fetchHosts, fetchSession } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import type { Level } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { NativeSelect } from '#lib/components/ui/native-select/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import Callout from '#lib/components/lor/Callout.svelte';
	import Card from '#lib/components/lor/Card.svelte';
	import ChoiceCard from '#lib/components/lor/ChoiceCard.svelte';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import DurationPicker from '#lib/components/lor/DurationPicker.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import PageSkeleton from '#lib/components/lor/PageSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import Warnings from '#lib/components/lor/Warnings.svelte';

	const qc = useQueryClient();
	const agents = createQuery(() => ({ queryKey: keys.agents, queryFn: fetchAgents }));
	const hosts = createQuery(() => ({ queryKey: keys.hosts, queryFn: fetchHosts }));
	const session = createQuery(() => ({ queryKey: keys.session, queryFn: fetchSession }));

	const usableAgents = $derived(
		(agents.data ?? []).filter((a) => a.status === 'active' && a.enrolled)
	);
	const usableHosts = $derived((hosts.data ?? []).filter((h) => !h.disabled && h.host_key));

	let agentId = $state<number | null>(null);
	let hostId = $state<number | null>(Number(page.url.searchParams.get('host_id')) || null);
	let level = $state<Level>('user');
	let duration = $state(3600);
	let durationValid = $state(true);
	let note = $state('');
	let confirmed = $state(false);
	let busy = $state(false);
	let message = $state<string | null>(null);
	let createdId = $state<number | null>(null);

	$effect(() => {
		if (agentId === null && usableAgents.length === 1) agentId = usableAgents[0].id;
	});

	const host = $derived(usableHosts.find((h) => h.id === hostId));
	const agent = $derived(usableAgents.find((a) => a.id === agentId));

	// Defaults follow the host: lor_infra hosts default to 15 minutes, and
	// level is reset when the host doesn't allow sudo.
	let lastHost = $state<number | null>(null);
	$effect(() => {
		if (host && host.id !== lastHost) {
			lastHost = host.id;
			if (host.max_level !== 'sudo') level = 'user';
			duration = host.lor_infra ? 900 : (session.data?.config?.default_grant_duration_s ?? 3600);
			confirmed = false;
		}
	});

	const sudo = $derived(level === 'sudo');
	const ready = $derived(!!host && !!agent && (!sudo || confirmed) && durationValid && !busy);
	const endsAt = $derived(clock.now + duration * 1000);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!host || !agent) return;
		busy = true;
		try {
			const res = await stepUp({
				kind: 'create_offer',
				agent_id: agent.id,
				host_id: host.id,
				level,
				duration_s: duration,
				confirm_sudo: sudo && confirmed,
				note
			});
			// Built here so the time is in the admin's zone (with UTC alongside).
			message = res.grant?.valid_before
				? offerChatLine(res.grant.host.name, res.grant.level, Date.parse(res.grant.valid_before))
				: (res.offer_message ?? null);
			createdId = res.grant?.id ?? null;
			toast.success(`Offer created for ${agent.name} on ${host.name}`);
			qc.invalidateQueries({ queryKey: keys.dashboard });
		} catch (err) {
			if (!isCancel(err)) toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}
</script>

<PageHeader
	title="Grant access"
	subtitle="Give an agent access before it asks. The timer starts now."
	back={{ href: '/', label: 'Home' }}
/>

{#if message}
	<div class="space-y-5">
		<Callout tone="success" title="Offer created">
			The agent claims it on its next <code>lor ssh</code> or <code>lor request</code>.
		</Callout>
		<Section title="Paste this into the chat with the agent">
			<Card><CopyField value={message} label="Copy message" /></Card>
		</Section>
		<div class="flex flex-wrap gap-2">
			{#if createdId}<Button href="/grants/{createdId}">View grant</Button>{/if}
			<Button variant="ghost" href="/">Done</Button>
		</div>
	</div>
{:else if agents.isPending || hosts.isPending}
	<PageSkeleton header={false} sections={2} />
{:else if agents.isError || hosts.isError}
	<ErrorState
		variant="inset"
		error={agents.error ?? hosts.error}
		onretry={() => {
			agents.refetch();
			hosts.refetch();
		}}
	/>
{:else if usableAgents.length === 0 || usableHosts.length === 0}
	<EmptyState
		icon={usableAgents.length === 0 ? Bot : Server}
		title={usableAgents.length === 0 ? 'No enrolled agent yet' : 'No host ready for access'}
		description={usableAgents.length === 0
			? 'Add an agent and run its enroll command first.'
			: 'A host needs to be enabled and have a scanned host key.'}
	>
		{#snippet action()}
			<Button variant="secondary" href={usableAgents.length === 0 ? '/agents' : '/hosts'}
				>{usableAgents.length === 0 ? 'Go to Agents' : 'Go to Hosts'}</Button
			>
		{/snippet}
	</EmptyState>
{:else}
	<form class="space-y-7" onsubmit={submit}>
		<!-- Locked while the passkey prompt is open: the action is already fixed. -->
		<fieldset disabled={busy} class="contents">
			<Section title="Who and where">
				<Card class="space-y-5">
					<Field label="Agent" for="agent">
						<NativeSelect id="agent" bind:value={agentId} required>
							<option value={null} disabled>Choose an agent</option>
							{#each usableAgents as a (a.id)}<option value={a.id}>{a.name}</option>{/each}
						</NativeSelect>
					</Field>
					<Field
						label="Host"
						for="host"
						hint="Only enabled hosts with a scanned host key are listed."
					>
						<NativeSelect id="host" bind:value={hostId} required>
							<option value={null} disabled>Choose a host</option>
							{#each usableHosts as h (h.id)}<option value={h.id}>{h.name} · {h.address}</option
								>{/each}
						</NativeSelect>
					</Field>
				</Card>
			</Section>

			{#if host?.max_level === 'sudo'}
				<Section title="Level">
					<div class="space-y-2" role="radiogroup" aria-label="Level">
						<ChoiceCard name="level" value="user" bind:group={level} title="Standard user">
							Logs in as <code>agent</code>, no sudo.
						</ChoiceCard>
						<ChoiceCard
							name="level"
							value="sudo"
							bind:group={level}
							title="sudo (root)"
							tone="warning"
						>
							Logs in as <code>agent-sudo</code>. Can read, change or delete anything on {host.name}.
						</ChoiceCard>
					</div>
				</Section>
			{/if}

			{#if host}<Warnings warnings={host.warnings} {level} />{/if}

			<Section title="Duration">
				<Card>
					<DurationPicker bind:value={duration} bind:valid={durationValid} id="offer-duration" />
				</Card>
			</Section>

			<Section>
				<Card>
					<Field label="Note" for="note" optional hint="For your records; shown on the grant.">
						<Textarea
							id="note"
							bind:value={note}
							rows={2}
							maxlength={1000}
							placeholder="What it's for"
						/>
					</Field>
				</Card>
			</Section>

			<Section title="Confirm">
				<Card class="space-y-4">
					{#if host && agent}
						<p data-testid="offer-summary" class="text-body">
							{agent.name} gets
							<strong class={sudo ? 'text-warning' : ''}
								>{sudo ? 'sudo (root)' : 'standard user'}</strong
							>
							access to <strong>{host.name}</strong> for {formatDuration(duration)}, until
							<span class="nums">{formatClock(endsAt, clock.now)}</span>.
						</p>
						{#if sudo}
							<label
								class="flex cursor-pointer items-start gap-3 rounded-lg border border-warning-edge bg-warning-tint px-3 py-2.5"
							>
								<Checkbox
									bind:checked={confirmed}
									id="confirm-sudo-offer"
									class="mt-0.5 border-warning bg-transparent"
								/>
								<span class="text-callout">I understand this gives root on {host.name}</span>
							</label>
						{/if}
					{:else}
						<p class="text-callout text-muted-foreground">Choose an agent and a host first.</p>
					{/if}
					<Button
						type="submit"
						size="lg"
						class="w-full"
						disabled={!ready}
						data-testid="create-offer"
					>
						{#if busy}<Spinner />{:else if host && agent}<KeyRound />{:else}<Send />{/if}
						{busy
							? 'Waiting for passkey…'
							: host && agent
								? `Grant ${sudo ? 'sudo' : 'standard user'} on ${host.name} for ${formatDuration(duration)}`
								: 'Grant access'}
					</Button>
				</Card>
			</Section>
		</fieldset>
	</form>
{/if}
