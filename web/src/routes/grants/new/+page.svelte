<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatClock, formatDuration } from '#lib/format.js';
	import { keys, fetchAgents, fetchHosts, fetchSession } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import type { Level } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import DurationPicker from '#lib/components/lor/DurationPicker.svelte';
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
			message = res.offer_message ?? null;
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
/>

{#if message}
	<Section title="Offer created">
		<p class="mb-3">Paste this into the chat with the agent:</p>
		<CopyField value={message} label="Copy message" />
		<div class="mt-4 flex gap-2">
			{#if createdId}<Button variant="secondary" href="/grants/{createdId}">View grant</Button>{/if}
			<Button variant="ghost" href="/">Done</Button>
		</div>
	</Section>
{:else}
	<form class="space-y-6" onsubmit={submit}>
		<Section>
			<div class="space-y-5">
				<div class="space-y-2">
					<Label for="agent">Agent</Label>
					<select
						id="agent"
						bind:value={agentId}
						class="h-10 w-full rounded-md border border-input bg-background px-3"
						required
					>
						<option value={null} disabled>Choose an agent</option>
						{#each usableAgents as a (a.id)}<option value={a.id}>{a.name}</option>{/each}
					</select>
					{#if agents.isSuccess && usableAgents.length === 0}
						<p class="text-sm text-muted-foreground">
							No enrolled agents yet. <a href="/agents">Add one</a>.
						</p>
					{/if}
				</div>
				<div class="space-y-2">
					<Label for="host">Host</Label>
					<select
						id="host"
						bind:value={hostId}
						class="h-10 w-full rounded-md border border-input bg-background px-3"
						required
					>
						<option value={null} disabled>Choose a host</option>
						{#each usableHosts as h (h.id)}<option value={h.id}>{h.name} · {h.address}</option
							>{/each}
					</select>
				</div>
				{#if host?.max_level === 'sudo'}
					<fieldset class="space-y-2">
						<legend class="mb-2 text-sm font-medium">Level</legend>
						<label class="flex gap-3"
							><input
								type="radio"
								bind:group={level}
								value="user"
								class="mt-1 size-4 accent-primary"
							/>
							<span
								>Standard user <span class="block text-sm text-muted-foreground"
									>Logs in as <code>agent</code>, no sudo.</span
								></span
							></label
						>
						<label class="flex gap-3"
							><input
								type="radio"
								bind:group={level}
								value="sudo"
								class="mt-1 size-4 accent-primary"
							/>
							<span
								>sudo (root) <span class="block text-sm text-muted-foreground"
									>Logs in as <code>agent-sudo</code>.</span
								></span
							></label
						>
					</fieldset>
				{/if}
			</div>
		</Section>

		{#if host}<Warnings warnings={host.warnings} {level} />{/if}

		<Section title="Duration">
			<DurationPicker bind:value={duration} bind:valid={durationValid} id="offer-duration" />
		</Section>

		<Section>
			<div class="space-y-2">
				<Label for="note">Note (optional)</Label>
				<Textarea
					id="note"
					bind:value={note}
					rows={2}
					maxlength={1000}
					placeholder="What it's for"
				/>
			</div>
		</Section>

		{#if host && agent}
			<Section title="Confirm">
				<p data-testid="offer-summary">
					{agent.name} gets <strong>{sudo ? 'sudo (root)' : 'standard user'}</strong> access to
					<strong>{host.name}</strong> for {formatDuration(duration)}, until {formatClock(
						endsAt,
						clock.now
					)}.
				</p>
				{#if sudo}
					<label class="mt-4 flex items-start gap-3">
						<Checkbox bind:checked={confirmed} id="confirm-sudo-offer" class="mt-0.5" />
						<span>I understand this gives root on {host.name}</span>
					</label>
				{/if}
				<Button
					type="submit"
					size="lg"
					class="mt-4 w-full"
					disabled={!ready}
					data-testid="create-offer"
				>
					<KeyRound />
					{busy
						? 'Waiting for passkey…'
						: `Grant ${sudo ? 'sudo' : 'standard user'} on ${host.name} for ${formatDuration(duration)}`}
				</Button>
			</Section>
		{/if}
	</form>
{/if}
