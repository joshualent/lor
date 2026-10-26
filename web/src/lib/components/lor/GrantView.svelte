<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import {
		formatAgo,
		formatArgv,
		formatClock,
		formatCountdown,
		formatDuration,
		formatTimestamp
	} from '#lib/format.js';
	import { keys, fetchGrant, LIVE_MS } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import { actionLabel } from '#lib/labels.js';
	import Countdown from './Countdown.svelte';
	import DurationPicker from './DurationPicker.svelte';
	import Warnings from './Warnings.svelte';
	import UnverifiedText from './UnverifiedText.svelte';
	import Confirm from './Confirm.svelte';
	import ExtendDialog from './ExtendDialog.svelte';
	import Row from './Row.svelte';
	import Section from './Section.svelte';
	import Status from './Status.svelte';

	let { id }: { id: number } = $props();
	const qc = useQueryClient();

	const q = createQuery(() => ({
		queryKey: keys.grant(id),
		queryFn: () => fetchGrant(id),
		refetchInterval: LIVE_MS
	}));
	const g = $derived(q.data);

	// Consent choices (initialized from the server's defaults once).
	let level = $state<Level>('user');
	let duration = $state(3600);
	let durationValid = $state(true);
	let confirmed = $state(false);
	let initialized = $state(false);
	$effect(() => {
		if (g?.consent && !initialized) {
			level = g.consent.default_level;
			duration = g.consent.default_duration_s;
			initialized = true;
		}
	});

	let busy = $state<string | null>(null);
	let denyOpen = $state(false);
	let revokeOpen = $state(false);
	let extendOpen = $state(false);
	let note = $state('');

	function refresh() {
		qc.invalidateQueries({ queryKey: keys.grants });
		qc.invalidateQueries({ queryKey: keys.dashboard });
	}

	async function run(name: string, fn: () => Promise<unknown>, ok: string) {
		busy = name;
		try {
			await fn();
			toast.success(ok);
			refresh();
			return true;
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
			refresh();
			return false;
		} finally {
			busy = null;
		}
	}

	const sudo = $derived(level === 'sudo');
	const canAuthorize = $derived(!!g?.actions.authorize && (!sudo || confirmed) && durationValid);
	const endsAt = $derived(clock.now + duration * 1000);

	async function authorize() {
		if (!g) return;
		await run(
			'authorize',
			() =>
				stepUp({
					kind: 'authorize_request',
					grant_id: g.id,
					level,
					duration_s: duration,
					confirm_sudo: sudo && confirmed
				}),
			`Authorized ${sudo ? 'sudo' : 'standard user access'} on ${g.host.name}`
		);
	}

	async function deny() {
		if (!g) return;
		if (
			await run(
				'deny',
				() =>
					call(api.POST('/grants/{id}/deny', { params: { path: { id: g.id } }, body: { note } })),
				'Request denied'
			)
		) {
			denyOpen = false;
			note = '';
		}
	}

	async function revoke() {
		if (!g) return;
		if (
			await run(
				'revoke',
				() =>
					call(api.POST('/grants/{id}/revoke', { params: { path: { id: g.id } }, body: { note } })),
				g.status === 'offered' ? 'Offer withdrawn' : 'Access revoked'
			)
		) {
			revokeOpen = false;
			note = '';
		}
	}

	const levelText = (l: string) => (l === 'sudo' ? 'sudo' : 'standard user');
	const isRequest = $derived(g?.origin === 'agent_request');
	const levelSummary = $derived(
		g
			? levelText(g.level) +
					(g.level !== g.requested_level ? ` (asked for ${levelText(g.requested_level)})` : '')
			: ''
	);
</script>

{#if q.isPending}
	<p class="text-muted-foreground">Loading…</p>
{:else if q.isError}
	<p class="text-destructive">{errorMessage(q.error)}</p>
{:else if g}
	<!-- ============================================================ consent view -->
	{#if g.consent && g.actions.authorize}
		<article class="space-y-6" data-testid="consent">
			<header>
				<h1 class="text-2xl leading-tight">
					<span class="font-normal text-muted-foreground">{g.agent.name} wants access to</span>
					{g.host.name}
				</h1>
				<p class="mt-1 text-sm text-muted-foreground">
					Request #{g.id} · {formatAgo(Date.parse(g.requested_at), clock.now)} · approve within
					{formatCountdown(Date.parse(g.consent.approval_deadline), clock.now)}
				</p>
			</header>

			<Warnings warnings={g.warnings} {level} />

			<Section>
				<dl class="text-[15px]">
					<Row label="Host"
						>{g.host.name} · <span class="font-mono text-sm">{g.host.address}</span></Row
					>
					<Row label="Agent"
						>{g.agent.name} <span class="text-muted-foreground">(enrolled agent)</span></Row
					>
					<Row label="Connect from"
						>only <span class="font-mono text-sm">{g.agent.lan_addrs.join(', ')}</span></Row
					>
					<Row label="Asked for"
						>{levelText(g.requested_level)} for {formatDuration(g.requested_duration_s ?? 0)}</Row
					>
				</dl>
			</Section>

			<Section title="Level">
				<div class="space-y-2" role="radiogroup" aria-label="Level">
					{#each g.consent.levels as l (l)}
						<label
							class="flex cursor-pointer gap-3 rounded-lg border p-3 {level === l
								? 'border-primary bg-primary/6'
								: 'border-border'}"
						>
							<input
								type="radio"
								name="level"
								value={l}
								bind:group={level}
								class="mt-1 size-4 accent-primary"
							/>
							<span>
								<span class="block font-medium"
									>{l === 'sudo' ? 'sudo (root)' : 'Standard user'}</span
								>
								<span class="block text-sm text-muted-foreground">
									{#if l === 'sudo'}
										Logs in as <code>agent-sudo</code>. Can read, change or delete anything on {g
											.host.name}.
									{:else}
										Logs in as <code>agent</code> (no sudo). Can run programs and read files any normal
										user can read. Can't install software or change system settings.
									{/if}
								</span>
							</span>
						</label>
					{/each}
				</div>
			</Section>

			<Section title="Duration">
				<DurationPicker bind:value={duration} bind:valid={durationValid} />
				<p class="mt-3 text-sm text-muted-foreground">
					→ ends {formatClock(endsAt, clock.now)} (extendable)
				</p>
			</Section>

			<Section title="Limits">
				<ul class="list-inside list-disc space-y-0.5 text-sm text-muted-foreground">
					{#each g.consent.limits as lim (lim)}<li>{lim}</li>{/each}
				</ul>
			</Section>

			<UnverifiedText text={g.reason} />

			<div
				class="sticky bottom-0 -mx-4 space-y-3 border-t bg-background/95 px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))] backdrop-blur sm:static sm:mx-0 sm:border-0 sm:px-0"
			>
				{#if sudo}
					<label class="flex items-start gap-3">
						<Checkbox bind:checked={confirmed} id="confirm-sudo" class="mt-0.5" />
						<span>I understand this gives root on {g.host.name}</span>
					</label>
				{/if}
				<div class="flex flex-col gap-2 sm:flex-row">
					<Button
						size="lg"
						class="sm:flex-1"
						disabled={!canAuthorize || busy !== null}
						onclick={authorize}
						data-testid="authorize"
					>
						<KeyRound />
						{busy === 'authorize'
							? 'Waiting for passkey…'
							: `Authorize ${sudo ? 'sudo' : 'standard user'} on ${g.host.name} for ${formatDuration(duration)}`}
					</Button>
					<Button
						size="lg"
						variant="outline"
						disabled={busy !== null}
						onclick={() => (denyOpen = true)}>Deny</Button
					>
				</div>
			</div>
		</article>
	{:else}
		<!-- ============================================================ decided / active view -->
		<article class="space-y-6" data-testid="grant">
			<header>
				<h1 class="text-2xl leading-tight">
					{g.host.name}
					<span class="font-normal text-muted-foreground">· {g.agent.name}</span>
				</h1>
				<p class="mt-1 text-sm text-muted-foreground">
					{isRequest ? 'Request' : 'Offer'} #{g.id} · {levelSummary}
				</p>
			</header>

			<Section>
				<dl>
					<Row label="Status"><Status status={g.status} /></Row>
					{#if g.status === 'active' || g.status === 'offered'}
						<Row label="Ends">
							{formatClock(Date.parse(g.valid_before!), clock.now)} · <Countdown
								end={g.valid_before}
							/>
						</Row>
					{:else if g.ended_at}
						<Row label="Ended">{formatTimestamp(Date.parse(g.ended_at))}</Row>
					{/if}
					{#if g.decided_by}<Row label="Decided by">{g.decided_by.replace(/^admin:/, '')}</Row>{/if}
					{#if g.decision_note && isRequest}<Row label="Note">{g.decision_note}</Row>{/if}
					<Row label="Host"
						>{g.host.name} · <span class="font-mono text-sm">{g.host.address}</span></Row
					>
					<Row label="Connect from"
						><span class="font-mono text-sm">{g.agent.lan_addrs.join(', ')}</span></Row
					>
					{#if g.public_key_fp}<Row label="Key"
							><span class="font-mono text-xs break-all">{g.public_key_fp}</span></Row
						>{/if}
				</dl>
				{#if g.actions.extend || g.actions.revoke}
					<div class="mt-4 flex gap-2">
						{#if g.actions.extend}<Button variant="secondary" onclick={() => (extendOpen = true)}
								>Extend</Button
							>{/if}
						{#if g.actions.revoke}<Button variant="destructive" onclick={() => (revokeOpen = true)}
								>{g.status === 'offered' ? 'Withdraw offer' : 'Revoke'}</Button
							>{/if}
					</div>
				{/if}
			</Section>

			{#if g.status === 'active' || g.status === 'offered'}
				<Warnings warnings={g.warnings} level={g.level} />
			{/if}

			{#if g.reason}
				<UnverifiedText
					text={g.reason}
					label={isRequest ? 'Reason (written by the agent, unverified)' : 'Note on the offer'}
				/>
			{/if}

			{#if g.pending_extension}
				<Section title="Extension requested">
					<p>
						+{formatDuration(g.pending_extension.requested_duration_s)} ·
						<a href="/e/{g.pending_extension.id}">Review the request</a>
					</p>
				</Section>
			{/if}

			<Section title="Command log">
				<p class="mb-3 text-sm text-muted-foreground">
					Reported by the agent's CLI. It shows what <code>lor ssh</code> was asked to run, not what actually
					happened.
				</p>
				{#if g.commands.length === 0}
					<p class="text-sm text-muted-foreground">No commands reported yet.</p>
				{:else}
					<ol class="space-y-2" data-testid="command-log">
						{#each g.commands as c (c.id)}
							<li class="grid grid-cols-[auto_1fr] gap-x-3 text-sm">
								<span class="text-muted-foreground tabular-nums"
									>{formatClock(Date.parse(c.reported_at), clock.now)}</span
								>
								<code class="break-all whitespace-pre-wrap">{formatArgv(c.argv)}</code>
							</li>
						{/each}
					</ol>
				{/if}
			</Section>

			{#if g.extensions.length}
				<Section title="Extension requests">
					<ul class="space-y-3">
						{#each g.extensions as x (x.id)}
							<li class="text-sm">
								<a href="/e/{x.id}">+{formatDuration(x.requested_duration_s)}</a> ·
								<Status status={x.status} />
							</li>
						{/each}
					</ul>
				</Section>
			{/if}

			<Section title="Certificates">
				{#if g.certificates.length === 0}
					<p class="text-sm text-muted-foreground">None issued.</p>
				{:else}
					<ul class="space-y-1 text-sm">
						{#each g.certificates as c (c.id)}
							<li>
								Serial {c.id} · until {formatClock(Date.parse(c.valid_before), clock.now)}
								<span class="text-muted-foreground">· {c.issued_by.replace(/^admin:/, '')}</span>
							</li>
						{/each}
					</ul>
				{/if}
			</Section>

			<Section title="Timeline">
				<ol class="space-y-1.5 text-sm">
					{#each g.timeline as ev (ev.id)}
						<li class="grid grid-cols-[auto_1fr] gap-x-3">
							<span class="text-muted-foreground tabular-nums"
								>{formatTimestamp(Date.parse(ev.ts))}</span
							>
							<span
								>{actionLabel(ev.action)}
								<span class="text-muted-foreground">· {ev.actor}</span></span
							>
						</li>
					{/each}
				</ol>
			</Section>
		</article>
	{/if}

	<Confirm
		bind:open={denyOpen}
		title="Deny this request?"
		description="The agent is told it was denied. You can add a note."
	>
		<div class="space-y-2">
			<Label for="deny-note">Note (optional)</Label>
			<Textarea id="deny-note" bind:value={note} maxlength={1000} rows={3} />
		</div>
		{#snippet footer()}
			<Button variant="outline" onclick={() => (denyOpen = false)}>Cancel</Button>
			<Button
				variant="destructive"
				disabled={busy !== null}
				onclick={deny}
				data-testid="confirm-deny">Deny</Button
			>
		{/snippet}
	</Confirm>

	<Confirm
		bind:open={revokeOpen}
		title={g.status === 'offered' ? 'Withdraw this offer?' : `Revoke access to ${g.host.name}?`}
		description="lor ssh stops working right away. A raw ssh session with the saved certificate could still work until it expires."
	>
		<div class="space-y-2">
			<Label for="revoke-note">Note (optional)</Label>
			<Textarea id="revoke-note" bind:value={note} maxlength={1000} rows={2} />
		</div>
		{#snippet footer()}
			<Button variant="outline" onclick={() => (revokeOpen = false)}>Cancel</Button>
			<Button variant="destructive" disabled={busy !== null} onclick={revoke}
				>{g.status === 'offered' ? 'Withdraw' : 'Revoke'}</Button
			>
		{/snippet}
	</Confirm>

	{#if g.valid_before}
		<ExtendDialog
			bind:open={extendOpen}
			grantId={g.id}
			host={g.host.name}
			end={g.valid_before}
			onDone={refresh}
		/>
	{/if}
{/if}
