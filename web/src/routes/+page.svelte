<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import Inbox from '@lucide/svelte/icons/inbox';
	import TimerReset from '@lucide/svelte/icons/timer-reset';
	import Clock from '@lucide/svelte/icons/clock';
	import ShieldCheck from '@lucide/svelte/icons/shield-check';
	import SquareTerminal from '@lucide/svelte/icons/square-terminal';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage } from '#lib/errors.js';
	import { formatAgo, formatArgv, formatCountdown, formatDuration } from '#lib/format.js';
	import { keys, fetchDashboard, LIVE_MS, type Summary } from '#lib/queries.js';
	import { hostLook } from '#lib/status.js';
	import { useNames } from '#lib/names.svelte.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { REVOKE_NOTE, WITHDRAW_NOTE } from '#lib/status.js';
	import AuditEventRow from '#lib/components/lor/AuditEventRow.svelte';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import Countdown from '#lib/components/lor/Countdown.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import ExtendDialog from '#lib/components/lor/ExtendDialog.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import LevelBadge from '#lib/components/lor/LevelBadge.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListRow from '#lib/components/lor/ListRow.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import PageSkeleton from '#lib/components/lor/PageSkeleton.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import StatusBadge from '#lib/components/lor/StatusBadge.svelte';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';

	const qc = useQueryClient();
	const q = createQuery(() => ({
		queryKey: keys.dashboard,
		queryFn: fetchDashboard,
		refetchInterval: LIVE_MS
	}));
	const d = $derived(q.data);
	const names = useNames();

	let extendFor = $state<Summary | null>(null);
	let extendOpen = $state(false);
	let revokeFor = $state<Summary | null>(null);
	let revokeOpen = $state(false);

	function refresh() {
		qc.invalidateQueries({ queryKey: keys.dashboard });
		qc.invalidateQueries({ queryKey: keys.grants });
	}

	async function revoke() {
		if (!revokeFor) return;
		revoking = true;
		try {
			await call(
				api.POST('/grants/{id}/revoke', { params: { path: { id: revokeFor.id } }, body: {} })
			);
			toast.success(
				revokeFor.status === 'offered'
					? 'Offer withdrawn'
					: `Revoked access to ${revokeFor.host.name}`
			);
			revokeOpen = false;
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			revoking = false;
		}
		refresh();
	}

	const pendingCount = $derived(
		(d?.pending_requests.length ?? 0) + (d?.pending_extensions.length ?? 0)
	);
	// Quick links to the sections ("4 waiting · 3 active · 1 offered").
	const summary = $derived.by(() => {
		if (!d) return [];
		return [
			{ href: '#waiting', n: pendingCount, label: 'waiting for you' },
			{ href: '#active', n: d.active.length, label: 'active' },
			{ href: '#offers', n: d.offered.length, label: 'offered' }
		].filter((x) => x.n > 0);
	});
	const hasCommands = $derived(!!d?.active.some((g) => g.last_command));
	let revoking = $state(false);
</script>

<PageHeader
	title="Home"
	subtitle={d && summary.length === 0 ? 'Nothing needs you right now.' : undefined}
>
	{#snippet actions()}<Button href="/grants/new"><Plus /> Grant access</Button>{/snippet}
	{#snippet meta()}
		{#if summary.length}
			<nav aria-label="Summary" class="-mt-2 flex flex-wrap gap-x-3 gap-y-1 text-callout">
				{#each summary as x (x.href)}
					<a
						href={x.href}
						class="-mx-1 rounded-md px-1 py-1.5 text-muted-foreground no-underline! focus-ring hover:text-foreground"
						><span class="font-medium text-foreground nums">{x.n}</span> {x.label}</a
					>
				{/each}
			</nav>
		{/if}
	{/snippet}
</PageHeader>

{#snippet grantActions(g: Summary, cls: string)}
	<div class={cls}>
		<Button
			size="sm"
			variant="secondary"
			onclick={() => {
				extendFor = g;
				extendOpen = true;
			}}
			aria-label="Extend {g.host.name}">Extend</Button
		>
		<Button
			size="sm"
			variant="ghost"
			class="text-destructive hover:text-destructive"
			onclick={() => {
				revokeFor = g;
				revokeOpen = true;
			}}
			aria-label="{g.status === 'offered' ? 'Withdraw offer for' : 'Revoke'} {g.host.name}"
			>{g.status === 'offered' ? 'Withdraw' : 'Revoke'}</Button
		>
	</div>
{/snippet}

{#if q.isError && q.data}<StaleNotice
		error={q.error}
		updatedAt={q.dataUpdatedAt}
		fetching={q.isFetching}
		onretry={() => q.refetch()}
	/>{/if}

{#if q.isPending}
	<PageSkeleton header={false} sections={3} />
{:else if q.isError && !q.data}
	<ErrorState error={q.error} fetching={q.isFetching} onretry={() => q.refetch()} />
{:else if d}
	<div class="space-y-8">
		{#if pendingCount > 0}
			<Section
				id="waiting"
				title="Waiting for you"
				count={pendingCount}
				description="Reasons are written by the agents and unverified."
			>
				<List data-testid="pending">
					{#each d.pending_requests as g (g.id)}
						<ListRow href="/r/{g.id}" density="relaxed">
							{#snippet leading()}<IconTile icon={Inbox} tone="warning" />{/snippet}
							{#snippet titleContent()}
								<span class="flex flex-wrap items-center gap-x-1.5 gap-y-1">
									<span>{g.agent.name}</span>
									<span class="font-normal text-muted-foreground">wants</span>
									<LevelBadge level={g.requested_level} short />
									<span class="font-normal text-muted-foreground">on</span>
									<span>{g.host.name}</span>
									{#each hostLook(g.host).badges as b (b.label)}<Badge tone={b.tone}
											>{b.label}</Badge
										>{/each}
								</span>
							{/snippet}
							{#snippet subtitleContent()}
								<span class="line-clamp-2">“{g.reason}”</span>
								<span class="mt-1 flex items-center gap-1.5 text-footnote">
									<Clock class="size-3.5 shrink-0" aria-hidden="true" />
									{[
										`for ${formatDuration(g.requested_duration_s ?? 0)}`,
										g.approval_deadline
											? `${formatCountdown(Date.parse(g.approval_deadline), clock.now)} to decide`
											: undefined
									]
										.filter(Boolean)
										.join(' · ')}
								</span>
							{/snippet}
						</ListRow>
					{/each}
					{#each d.pending_extensions as x (x.extension.id)}
						<ListRow href="/e/{x.extension.id}" density="relaxed">
							{#snippet leading()}<IconTile icon={TimerReset} tone="warning" />{/snippet}
							{#snippet titleContent()}
								{x.grant.agent.name}
								<span class="font-normal text-muted-foreground">wants more time on</span>
								{x.grant.host.name}
								{#each hostLook(x.grant.host).badges as b (b.label)}<Badge
										tone={b.tone}
										class="ml-1">{b.label}</Badge
									>{/each}
							{/snippet}
							{#snippet subtitleContent()}
								<span class="line-clamp-2">“{x.extension.reason}”</span>
								<span class="mt-1 flex items-center gap-1.5 text-footnote">
									<Clock class="size-3.5 shrink-0" aria-hidden="true" />
									+{formatDuration(x.extension.requested_duration_s)}
								</span>
							{/snippet}
						</ListRow>
					{/each}
				</List>
			</Section>
		{/if}

		{#snippet grantRow(g: Summary)}
			{@const look = hostLook(g.host)}
			<ListRow
				href="/grants/{g.id}"
				label="{g.host.name}, {g.agent.name}{g.level === 'sudo' ? ', sudo' : ''}{g.status ===
				'offered'
					? ', offer not claimed yet'
					: ''}"
			>
				{#snippet leading()}<IconTile icon={look.icon} tone={look.tone} />{/snippet}
				{#snippet titleContent()}
					<span class="flex flex-wrap items-center gap-x-2 gap-y-1">
						{g.host.name}
						{#if g.level === 'sudo'}<LevelBadge level="sudo" short />{/if}
					</span>
				{/snippet}
				{#snippet subtitleContent()}
					{[
						g.agent.name,
						g.status === 'offered' ? 'not claimed yet' : undefined,
						g.last_command
							? `ran a command ${formatAgo(Date.parse(g.last_command.reported_at), clock.now)}`
							: undefined
					]
						.filter(Boolean)
						.join(' · ')}
				{/snippet}
				{#snippet metaContent()}
					{#if g.status === 'offered'}<StatusBadge status="offered" />{/if}
					<Countdown end={g.valid_before} variant="badge" />
				{/snippet}
				{#snippet trailing()}
					{@render grantActions(g, 'hidden gap-1 sm:flex')}
				{/snippet}
				{#if g.last_command}
					<p class="-mt-1.5 flex min-w-0 gap-1.5 pr-4 pb-3 pl-16 text-footnote">
						<SquareTerminal
							class="mt-0.5 size-3.5 shrink-0 text-icon-muted"
							aria-label="Last command"
						/>
						<code class="line-clamp-2 min-w-0 wrap-anywhere text-foreground/85"
							>{formatArgv(g.last_command.argv)}</code
						>
					</p>
				{/if}
				{@render grantActions(g, 'flex gap-1 px-4 pb-3 pl-16 sm:hidden')}
			</ListRow>
		{/snippet}

		<Section
			id="active"
			title="Active"
			count={d.active.length || undefined}
			description={hasCommands
				? 'Last commands are reported by the agents and unverified.'
				: undefined}
		>
			{#if d.active.length === 0}
				<EmptyState
					icon={ShieldCheck}
					title="No agent has access"
					description="Access starts when you approve a request or grant it ahead of time."
				>
					{#snippet action()}
						{#if d.offered.length === 0}
							<Button variant="secondary" href="/grants/new"><Plus /> Grant access</Button>
						{/if}
					{/snippet}
				</EmptyState>
			{:else}
				<List data-testid="active-grants">
					{#each d.active as g (g.id)}{@render grantRow(g)}{/each}
				</List>
			{/if}
		</Section>

		{#if d.offered.length}
			<Section
				id="offers"
				title="Offers"
				count={d.offered.length}
				footnote="Pre-approved access. The agent claims it on its next lor ssh or lor request."
			>
				<List data-testid="offers">
					{#each d.offered as g (g.id)}{@render grantRow(g)}{/each}
				</List>
			</Section>
		{/if}

		<Section title="Recent activity">
			{#snippet action()}
				<Button variant="ghost" size="sm" href="/audit" class="text-muted-foreground"
					>View all</Button
				>
			{/snippet}
			{#if d.recent_audit.length === 0}
				<EmptyState
					title="Nothing yet"
					description="Requests, approvals and changes show up here."
				/>
			{:else}
				<List ordered label="Recent activity">
					{#each d.recent_audit.slice(0, 8) as ev (ev.id)}
						<AuditEventRow event={ev} time="ago" names={names.current} />
					{/each}
				</List>
			{/if}
		</Section>
	</div>

	{#if extendFor?.valid_before}
		<ExtendDialog
			bind:open={extendOpen}
			grantId={extendFor.id}
			host={extendFor.host.name}
			end={extendFor.valid_before}
			onDone={refresh}
		/>
	{/if}
	<Confirm
		bind:open={revokeOpen}
		title={revokeFor?.status === 'offered'
			? 'Withdraw this offer?'
			: `Revoke access to ${revokeFor?.host.name}?`}
		description={revokeFor?.status === 'offered' ? WITHDRAW_NOTE : REVOKE_NOTE}
	>
		{#snippet footer()}
			<Button variant="outline" onclick={() => (revokeOpen = false)}>Cancel</Button>
			<Button variant="destructive" disabled={revoking} onclick={revoke}
				>{#if revoking}<Spinner />{/if}{revokeFor?.status === 'offered'
					? 'Withdraw'
					: 'Revoke'}</Button
			>
		{/snippet}
	</Confirm>
{/if}
