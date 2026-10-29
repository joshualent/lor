<script lang="ts">
	// A grant: the consent view while a request is pending, otherwise its
	// status, time left, command log, certificates and timeline. Used by
	// /r/[id] (approval links) and /grants/[id].
	import KeyRound from '@lucide/svelte/icons/key-round';
	import TimerReset from '@lucide/svelte/icons/timer-reset';
	import FileBadge from '@lucide/svelte/icons/file-badge';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatClock, formatCountdown, formatDuration } from '#lib/format.js';
	import { keys, fetchGrant, LIVE_MS } from '#lib/queries.js';
	import { hostLook, passkeyName, REVOKE_NOTE, WITHDRAW_NOTE } from '#lib/status.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import ActionBar from './ActionBar.svelte';
	import Callout from './Callout.svelte';
	import Card from './Card.svelte';
	import ChoiceCard from './ChoiceCard.svelte';
	import CommandLog from './CommandLog.svelte';
	import Confirm from './Confirm.svelte';
	import Countdown from './Countdown.svelte';
	import DescriptionItem from './DescriptionItem.svelte';
	import DescriptionList from './DescriptionList.svelte';
	import DurationPicker from './DurationPicker.svelte';
	import ErrorState from './ErrorState.svelte';
	import ExtendDialog from './ExtendDialog.svelte';
	import IconTile from './IconTile.svelte';
	import LevelBadge from './LevelBadge.svelte';
	import List from './List.svelte';
	import ListRow from './ListRow.svelte';
	import PageHeader from './PageHeader.svelte';
	import PageSkeleton from './PageSkeleton.svelte';
	import Section from './Section.svelte';
	import StaleNotice from './StaleNotice.svelte';
	import StatusBadge from './StatusBadge.svelte';
	import TimeBar from './TimeBar.svelte';
	import Timeline from './Timeline.svelte';
	import UnverifiedText from './UnverifiedText.svelte';
	import Warnings from './Warnings.svelte';

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
	const kindLabel = 'Grant';
	const levelSummary = $derived(
		g
			? levelText(g.level) +
					(g.level !== g.requested_level ? ` (asked for ${levelText(g.requested_level)})` : '')
			: ''
	);
	const live = $derived(g?.status === 'active' || g?.status === 'offered');
	const host = $derived(g ? hostLook(g.host) : null);
	// End of the newest certificate (it stays valid after a revoke; no KRL yet).
	const certEnd = $derived(
		g?.certificates.length ? Math.max(...g.certificates.map((c) => Date.parse(c.valid_before))) : 0
	);
</script>

{#if q.isPending}
	<PageSkeleton />
{:else if q.isError && !q.data}
	<ErrorState
		error={q.error}
		fetching={q.isFetching}
		onretry={() => q.refetch()}
		back={{ href: '/', label: 'Home' }}
	/>
{:else if g && host}
	{#if q.isError && q.data}<StaleNotice
			error={q.error}
			updatedAt={q.dataUpdatedAt}
			fetching={q.isFetching}
			onretry={() => q.refetch()}
		/>{/if}
	<!-- ============================================================ consent view -->
	{#if g.consent && g.actions.authorize}
		<article class="space-y-7" data-testid="consent">
			<PageHeader
				title="{g.agent.name} wants access to {g.host.name}"
				back={{ href: '/', label: 'Home' }}
			>
				{#snippet leading()}<IconTile icon={host.icon} tone={host.tone} size="lg" />{/snippet}
				{#snippet heading()}
					<span class="block text-callout font-normal tracking-normal text-muted-foreground"
						>{g.agent.name} wants access to</span
					>{g.host.name}
				{/snippet}
				{#snippet meta()}
					<StatusBadge status={g.status} />
					<Badge tone="neutral"
						>Approve within {formatCountdown(
							Date.parse(g.consent!.approval_deadline),
							clock.now
						)}</Badge
					>
					<span class="text-footnote text-muted-foreground"
						>Request #{g.id} · {formatAgo(Date.parse(g.requested_at), clock.now)}</span
					>
					<Button
						href="#agent-reason"
						variant="secondary"
						class="min-h-11 text-footnote sm:bg-transparent sm:px-0 sm:text-primary sm:hover:bg-transparent sm:hover:underline"
						>Read full reason</Button
					>
					<div
						class="flex w-full items-center justify-between gap-3 border-t border-separator pt-2 text-callout"
					>
						<div class="min-w-0">
							<p>
								Selected: <strong>{level === 'sudo' ? 'sudo' : 'standard user'}</strong> · {durationValid
									? formatDuration(duration)
									: 'Choose a duration'}
							</p>
							{#if durationValid && duration !== g.requested_duration_s}<p
									class="text-footnote text-muted-foreground"
								>
									Requested {formatDuration(g.requested_duration_s ?? 0)}
								</p>{/if}
						</div>
						<Button variant="link" size="sm" href="#access-choices" class="min-h-11 shrink-0"
							>Change access</Button
						>
					</div>
				{/snippet}
			</PageHeader>

			<Warnings warnings={g.warnings} {level} />

			<section
				id="agent-reason"
				tabindex="-1"
				aria-label="Full agent reason, unverified"
				class="rounded-xl focus-ring"
			>
				<UnverifiedText text={g.reason} />
			</section>

			<Section title="Request">
				<DescriptionList>
					<DescriptionItem label="Host">
						{g.host.name}
						<span class="font-mono text-callout text-muted-foreground">{g.host.address}</span>
					</DescriptionItem>
					<DescriptionItem label="Agent">{g.agent.name}</DescriptionItem>
					<DescriptionItem label="Connect from">
						only <span class="font-mono text-callout">{g.agent.lan_addrs.join(', ')}</span>
					</DescriptionItem>
					<DescriptionItem label="Asked for">
						{levelText(g.requested_level)} for {formatDuration(g.requested_duration_s ?? 0)}
					</DescriptionItem>
				</DescriptionList>
			</Section>

			<Section id="access-choices" title="Level">
				<div class="space-y-2" role="radiogroup" aria-label="Level">
					{#each g.consent.levels as l (l)}
						{#if l === 'sudo'}
							<ChoiceCard
								name="level"
								value="sudo"
								bind:group={level}
								title="sudo (root)"
								tone="warning"
							>
								Logs in as <code>agent-sudo</code>. Can read, change or delete anything on {g.host
									.name}.
							</ChoiceCard>
						{:else}
							<ChoiceCard name="level" value="user" bind:group={level} title="Standard user">
								Logs in as <code>agent</code> (no sudo). Can run programs and read files any normal user
								can read. Can't install software or change system settings.
							</ChoiceCard>
						{/if}
					{/each}
				</div>
			</Section>

			<Section title="Duration">
				<Card>
					<DurationPicker bind:value={duration} bind:valid={durationValid} />
					{#if g.requested_duration_s && g.consent.default_duration_s !== g.requested_duration_s}
						<p class="mt-3 text-footnote text-muted-foreground" data-testid="duration-note">
							Asked for {formatDuration(g.requested_duration_s)}. Lor suggests {formatDuration(
								g.consent.default_duration_s
							)}{g.host.lor_infra ? ' for Lor infrastructure' : ''}.
						</p>
					{/if}
					<p class="mt-3 border-t border-separator pt-3 text-callout text-muted-foreground">
						{#if durationValid}
							Ends <span class="font-medium text-foreground nums"
								>{formatClock(endsAt, clock.now)}</span
							> · extendable
						{:else}
							Ends when you enter a duration
						{/if}
					</p>
				</Card>
			</Section>

			<div class="compact-actions-only">
				<Button href="#grant-decision" variant="link" size="sm">Go to decision</Button>
			</div>

			<Section title="Limits">
				<ul class="space-y-2 rounded-xl bg-card px-4 py-3">
					{#each g.consent.limits as lim (lim)}
						<li class="flex gap-2.5 text-callout text-muted-foreground">
							<span class="mt-2 size-1 shrink-0 rounded-full bg-icon-muted" aria-hidden="true"
							></span>{lim}
						</li>
					{/each}
				</ul>
			</Section>

			<ActionBar id="grant-decision">
				{#snippet note()}
					{#if sudo}
						<label
							class="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-warning-edge bg-warning-tint px-3 py-2"
						>
							<Checkbox
								bind:checked={confirmed}
								id="confirm-sudo"
								class="border-warning bg-transparent"
							/>
							<span class="text-callout">I understand this gives root on {g.host.name}</span>
						</label>
					{/if}
				{/snippet}
				<Button
					size="lg"
					variant="outline"
					disabled={busy !== null}
					onclick={() => (denyOpen = true)}>Deny</Button
				>
				<Button
					size="lg"
					class="h-auto min-h-12 flex-1 py-2 leading-snug whitespace-normal"
					disabled={!canAuthorize || busy !== null}
					onclick={authorize}
					data-testid="authorize"
				>
					{#if busy === 'authorize'}<Spinner />{:else}<KeyRound />{/if}
					{busy === 'authorize'
						? 'Waiting for passkey…'
						: durationValid
							? `Authorize ${sudo ? 'sudo' : 'standard user'} on ${g.host.name} for ${formatDuration(duration)}`
							: `Authorize ${sudo ? 'sudo' : 'standard user'} on ${g.host.name}: enter a duration`}
				</Button>
			</ActionBar>
		</article>
	{:else}
		<!-- ============================================================ decided / active view -->
		<article class="space-y-7" data-testid="grant">
			<PageHeader
				title={g.host.name}
				subtitle="{g.agent.name} · {kindLabel} #{g.id}"
				back={{ href: '/', label: 'Home' }}
				documentTitle="{g.host.name} · {kindLabel} #{g.id}"
			>
				{#snippet leading()}<IconTile icon={host.icon} tone={host.tone} size="lg" />{/snippet}
				{#snippet meta()}
					<StatusBadge status={g.status} />
					<LevelBadge level={g.level} />
				{/snippet}
			</PageHeader>

			{#if (g.status === 'revoked' || g.status === 'released') && certEnd && certEnd > clock.now}
				<Callout
					tone="neutral"
					title={g.status === 'revoked' ? 'Revoked' : 'Released by the agent'}
				>
					lor ssh no longer works for this grant. The last certificate stays valid until {formatClock(
						certEnd,
						clock.now
					)}, so a raw ssh session with a saved copy could still log in until then.
				</Callout>
			{/if}

			{#if live && g.valid_before}
				<Card padding="lg">
					<div class="flex flex-wrap items-end justify-between gap-x-4 gap-y-1">
						<div>
							<p class="text-footnote text-muted-foreground">
								{g.status === 'offered' ? 'Offer open, not claimed yet' : 'Access ends in'}
							</p>
							<p class="text-title nums">
								<Countdown end={g.valid_before} suffix="" />
							</p>
						</div>
						<p class="text-callout text-muted-foreground">
							Ends {formatClock(Date.parse(g.valid_before), clock.now)}
						</p>
					</div>
					{#if g.valid_after}
						<TimeBar
							start={g.valid_after}
							end={g.valid_before}
							offer={g.status === 'offered'}
							class="mt-3"
						/>
					{/if}
					{#if g.actions.extend || g.actions.revoke}
						<div class="mt-4 flex gap-2">
							{#if g.actions.extend}<Button
									variant="secondary"
									class="flex-1 sm:flex-none"
									onclick={() => (extendOpen = true)}><TimerReset /> Extend</Button
								>{/if}
							{#if g.actions.revoke}<Button
									variant="destructive"
									class="flex-1 sm:flex-none"
									onclick={() => (revokeOpen = true)}
									>{g.status === 'offered' ? 'Withdraw offer' : 'Revoke'}</Button
								>{/if}
						</div>
					{/if}
				</Card>
				<Warnings warnings={g.warnings} level={g.level} />
			{/if}

			{#if g.pending_extension}
				<Callout tone="warning" icon={TimerReset} title="Extension requested">
					{g.agent.name} asks for +{formatDuration(g.pending_extension.requested_duration_s)}.
					{#snippet action()}
						<Button size="sm" href="/e/{g.pending_extension!.id}">Review the request</Button>
					{/snippet}
				</Callout>
			{/if}

			{#if g.certificates.length || g.commands.length}
				<Section
					title="Command log"
					count={g.commands.length || undefined}
					description="Unverified: reported by the agent's CLI. It shows what lor ssh was asked to run, not what actually happened."
				>
					<CommandLog commands={g.commands} ended={!live} />
				</Section>
			{/if}

			<Section title="Details">
				<DescriptionList>
					<DescriptionItem label="Level">{levelSummary}</DescriptionItem>
					{#if !live && g.ended_at}
						<DescriptionItem label="Ended"
							>{formatClock(Date.parse(g.ended_at), clock.now)}</DescriptionItem
						>
					{/if}
					{#if g.decided_by}<DescriptionItem label="Decided by"
							>{passkeyName(g.decided_by)}</DescriptionItem
						>{/if}
					{#if g.decision_note && isRequest}<DescriptionItem label="Note" layout="stacked"
							>{g.decision_note}</DescriptionItem
						>{/if}
					<DescriptionItem label="Host">
						{g.host.name}
						<span class="font-mono text-callout text-muted-foreground">{g.host.address}</span>
					</DescriptionItem>
					<DescriptionItem label="Connect from" mono>{g.agent.lan_addrs.join(', ')}</DescriptionItem
					>
					{#if g.public_key_fp}<DescriptionItem label="Key" layout="stacked" mono
							>{g.public_key_fp}</DescriptionItem
						>{/if}
				</DescriptionList>
			</Section>

			{#if g.reason}
				<UnverifiedText
					text={g.reason}
					label={isRequest ? 'Reason (written by the agent, unverified)' : 'Note on the offer'}
				/>
			{/if}

			{#if g.extensions.length}
				<Section title="Extension requests">
					<List>
						{#each g.extensions as x (x.id)}
							<ListRow
								href="/e/{x.id}"
								title="+{formatDuration(x.requested_duration_s)}"
								subtitle={formatAgo(Date.parse(x.requested_at), clock.now)}
							>
								{#snippet leading()}<IconTile icon={TimerReset} size="sm" />{/snippet}
								{#snippet metaContent()}<StatusBadge status={x.status} kind="extension" />{/snippet}
							</ListRow>
						{/each}
					</List>
				</Section>
			{/if}

			<Section title="Certificates" count={g.certificates.length || undefined}>
				{#if g.certificates.length === 0}
					<p class="rounded-xl bg-card px-4 py-3.5 text-callout text-muted-foreground">
						None issued{g.status === 'offered'
							? ' yet: the agent claims the offer on first use'
							: ''}.
					</p>
				{:else}
					<List>
						{#each g.certificates as c (c.id)}
							<ListRow
								title="Serial {c.id}"
								subtitle="Issued by {passkeyName(c.issued_by)}"
								meta="until {formatClock(Date.parse(c.valid_before), clock.now)}"
								density="compact"
							>
								{#snippet leading()}<IconTile icon={FileBadge} size="sm" />{/snippet}
							</ListRow>
						{/each}
					</List>
				{/if}
			</Section>

			<Section title="Timeline">
				<Timeline events={g.timeline} />
			</Section>
		</article>
	{/if}

	<Confirm
		bind:open={denyOpen}
		title="Deny this request?"
		description="The agent is told it was denied. You can add a note."
	>
		<p class="mb-4 text-callout">
			{g.agent.name} on {g.host.name} · {g.requested_level === 'sudo'
				? 'sudo (root)'
				: 'standard user'} · Request #{g.id}
		</p>
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
		description={g.status === 'offered' ? WITHDRAW_NOTE : REVOKE_NOTE}
	>
		<p class="mb-4 text-callout">
			{g.agent.name} on {g.host.name} · {g.level === 'sudo' ? 'sudo (root)' : 'standard user'} · Grant
			#{g.id}
		</p>
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
			agent={g.agent.name}
			level={g.level}
			end={g.valid_before}
			onDone={refresh}
		/>
	{/if}
{/if}
