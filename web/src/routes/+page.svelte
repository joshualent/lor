<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import Send from '@lucide/svelte/icons/send';
	import SummaryCard from '#lib/components/lor/SummaryCard.svelte';
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
	const activeByExpiry = $derived(
		[...(d?.active ?? [])].sort(
			(a, b) => Date.parse(a.valid_before ?? '') - Date.parse(b.valid_before ?? '')
		)
	);
	const expiring = $derived(
		activeByExpiry.filter(
			(g) =>
				g.valid_before &&
				Date.parse(g.valid_before) > clock.now &&
				Date.parse(g.valid_before) - clock.now <= 5 * 60_000
		)
	);
	const hasCommands = $derived(!!d?.active.some((g) => g.last_command));
	let revoking = $state(false);
</script>

<PageHeader title="Home" subtitle="Review requests and keep track of temporary access.">
	{#snippet actions()}<Button href="/grants/new"><Plus /> Grant access</Button>{/snippet}
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
			aria-label="Extend {g.host.name} for {g.agent.name}, {g.level === 'sudo'
				? 'sudo'
				: 'standard user'}, grant #{g.id}">Extend</Button
		>
		<Button
			size="sm"
			variant="ghost"
			class="text-destructive hover:text-destructive"
			onclick={() => {
				revokeFor = g;
				revokeOpen = true;
			}}
			aria-label="{g.status === 'offered' ? 'Withdraw offer for' : 'Revoke'} {g.host.name} for {g
				.agent.name}, {g.level === 'sudo' ? 'sudo' : 'standard user'}, grant #{g.id}"
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
	<PageSkeleton header={false} layout="dashboard" />
{:else if q.isError && !q.data}
	<ErrorState error={q.error} fetching={q.isFetching} onretry={() => q.refetch()} />
{:else if d}
	<nav class="mb-8 grid grid-cols-3 gap-2 sm:gap-4" aria-label="Summary">
		<SummaryCard
			href="#waiting"
			label="Waiting for you"
			value={pendingCount}
			description="Requests to review"
			icon={Inbox}
			tone="warning"
		/>
		<SummaryCard
			href="#active"
			label="Active access"
			value={d.active.length}
			description="Grants in progress"
			icon={ShieldCheck}
			tone="success"
		/>
		<SummaryCard
			href="#offers"
			label="Open offers"
			value={d.offered.length}
			description="Ready for an agent to claim"
			icon={Send}
			tone="info"
		/>
	</nav>
	{#if expiring.length}
		<div
			class="-mt-3 mb-6 flex min-h-11 flex-wrap items-center gap-x-2 gap-y-1 px-1 text-callout text-warning"
		>
			<Clock class="size-4" aria-hidden="true" /><span
				>{expiring[0].agent.name}'s access to {expiring[0].host.name} expires in <Countdown
					end={expiring[0].valid_before}
					suffix=""
				/>{expiring.length > 1 ? ` · ${expiring.length} expiring soon` : ''}</span
			>
			<Button variant="link" size="sm" href="/grants/{expiring[0].id}" class="ml-auto"
				>Review access</Button
			>
		</div>
	{/if}
	<div class="grid items-start gap-8 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
		<div class="min-w-0 space-y-8 xl:col-start-1 xl:row-start-1">
			{#if pendingCount > 0}
				<Section
					id="waiting"
					title="Waiting for you"
					count={pendingCount}
					description="Reasons are written by the agents and unverified."
				>
					<List data-testid="pending">
						{#each d.pending_requests as g (g.id)}
							<ListRow
								compactOnNarrow
								href="/r/{g.id}"
								density="relaxed"
								label="Review {g.agent.name} request for {g.requested_level === 'sudo'
									? 'sudo'
									: 'standard user'} on {g.host.name}, grant #{g.id}{g.approval_deadline
									? `, ${formatCountdown(Date.parse(g.approval_deadline), clock.now)} to decide`
									: ''}"
							>
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
							<ListRow
								compactOnNarrow
								href="/e/{x.extension.id}"
								density="relaxed"
								label="Review {x.grant.agent.name} extension request on {x.grant.host
									.name}, grant #{x.grant.id}, add {formatDuration(
									x.extension.requested_duration_s
								)}"
							>
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
			{:else}
				<Section id="waiting" title="Waiting for you" count={0}>
					<EmptyState
						icon={Inbox}
						size="compact"
						title="You're all caught up"
						description="New access requests and extensions appear here for your review."
					/>
				</Section>
			{/if}
		</div>
		<div class="min-w-0 space-y-8 xl:col-start-2 xl:row-span-2 xl:row-start-1">
			{#snippet grantRow(g: Summary)}
				{@const look = hostLook(g.host)}
				<ListRow
					stackMeta
					href="/grants/{g.id}"
					label="{g.host.name}, {g.agent.name}, {g.level === 'sudo'
						? 'sudo'
						: 'standard user'}, grant #{g.id}{g.status === 'offered'
						? ', offer not claimed'
						: ', active'}{g.valid_before
						? `, ${formatCountdown(Date.parse(g.valid_before), clock.now)} remaining`
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
					{@render grantActions(g, 'flex gap-3 px-4 pt-1 pb-4 pl-16')}
				</ListRow>
			{/snippet}

			<Section
				id="active"
				title="Active"
				count={d.active.length}
				description={hasCommands
					? 'Last commands are reported by the agents and unverified.'
					: undefined}
			>
				{#if d.active.length === 0}
					<EmptyState
						icon={ShieldCheck}
						size="compact"
						title="No agent has access"
						description="Access starts when you approve a request or grant it ahead of time."
					></EmptyState>
				{:else}
					<List data-testid="active-grants">
						{#each activeByExpiry as g (g.id)}{@render grantRow(g)}{/each}
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
			{:else}
				<Section id="offers" title="Offers" count={0}
					><p class="text-callout text-muted-foreground">
						No open offers. <Button variant="link" size="sm" href="/grants/new">Grant access</Button
						> before an agent asks.
					</p></Section
				>
			{/if}
		</div>
		<Section class="xl:col-start-1 xl:row-start-2" title="Recent activity">
			{#snippet action()}
				<Button variant="link" size="sm" href="/audit">View all</Button>
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
			agent={extendFor.agent.name}
			level={extendFor.level}
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
		{#if revokeFor}<p class="text-callout">
				{revokeFor.agent.name} on {revokeFor.host.name} · {revokeFor.level === 'sudo'
					? 'sudo (root)'
					: 'standard user'} · Grant #{revokeFor.id}
			</p>{/if}
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
