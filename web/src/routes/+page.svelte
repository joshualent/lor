<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage } from '#lib/errors.js';
	import { formatAgo, formatArgv, formatCountdown, formatDuration } from '#lib/format.js';
	import { keys, fetchDashboard, LIVE_MS, type Summary } from '#lib/queries.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import Countdown from '#lib/components/lor/Countdown.svelte';
	import ExtendDialog from '#lib/components/lor/ExtendDialog.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import { actionLabel } from '#lib/labels.js';

	const qc = useQueryClient();
	const q = createQuery(() => ({
		queryKey: keys.dashboard,
		queryFn: fetchDashboard,
		refetchInterval: LIVE_MS
	}));
	const d = $derived(q.data);

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
		}
		refresh();
	}

	const level = (l: string) => (l === 'sudo' ? 'sudo' : 'user');
	const pendingCount = $derived(
		(d?.pending_requests.length ?? 0) + (d?.pending_extensions.length ?? 0)
	);
</script>

<div class="mb-6 flex items-center justify-between gap-3">
	<h1 class="text-2xl">Home</h1>
	<Button href="/grants/new"><Plus /> Grant access</Button>
</div>

{#if q.isPending}
	<p class="text-muted-foreground">Loading…</p>
{:else if q.isError}
	<p class="text-destructive">{errorMessage(q.error)}</p>
{:else if d}
	<div class="space-y-6">
		{#if pendingCount > 0}
			<Section title="Waiting for you">
				<ul class="-my-2 divide-y divide-border">
					{#each d.pending_requests as g (g.id)}
						<li>
							<a href="/r/{g.id}" class="group flex items-center gap-3 py-3 no-underline!">
								<div class="min-w-0 flex-1">
									<p>
										<span class="font-medium">{g.agent.name}</span>
										<span class="text-muted-foreground">wants</span>
										<span class={g.requested_level === 'sudo' ? 'text-warning' : ''}
											>{level(g.requested_level)}</span
										>
										<span class="text-muted-foreground">on</span>
										<span class="font-medium">{g.host.name}</span>
										<span class="text-muted-foreground"
											>· {formatDuration(g.requested_duration_s ?? 0)}</span
										>
									</p>
									<p class="line-clamp-2 text-sm break-words text-muted-foreground">{g.reason}</p>
									{#if g.approval_deadline}
										<p class="text-xs text-muted-foreground">
											approve within {formatCountdown(Date.parse(g.approval_deadline), clock.now)}
										</p>
									{/if}
								</div>
								<ChevronRight class="size-4 shrink-0 text-muted-foreground" />
							</a>
						</li>
					{/each}
					{#each d.pending_extensions as x (x.extension.id)}
						<li>
							<a href="/e/{x.extension.id}" class="flex items-center gap-3 py-3 no-underline!">
								<div class="min-w-0 flex-1">
									<p>
										<span class="font-medium">{x.grant.agent.name}</span>
										<span class="text-muted-foreground">wants</span>
										+{formatDuration(x.extension.requested_duration_s)}
										<span class="text-muted-foreground">on</span>
										<span class="font-medium">{x.grant.host.name}</span>
									</p>
									<p class="line-clamp-2 text-sm break-words text-muted-foreground">
										{x.extension.reason}
									</p>
								</div>
								<ChevronRight class="size-4 shrink-0 text-muted-foreground" />
							</a>
						</li>
					{/each}
				</ul>
			</Section>
		{/if}

		<Section title="Active">
			{#if d.active.length === 0 && d.offered.length === 0}
				<p class="text-muted-foreground">No agent has access right now.</p>
			{:else}
				<ul class="-my-2 divide-y divide-border">
					{#each [...d.active, ...d.offered] as g (g.id)}
						<li class="py-3">
							<div class="flex items-start gap-3">
								<a href="/grants/{g.id}" class="min-w-0 flex-1 no-underline!">
									<p>
										<span class="font-medium">{g.host.name}</span>
										<span class="text-muted-foreground">· {g.agent.name} ·</span>
										<span class={g.level === 'sudo' ? 'text-warning' : ''}>{level(g.level)}</span>
										{#if g.status === 'offered'}<span class="text-muted-foreground"
												>· offered, not claimed yet</span
											>{/if}
									</p>
									<p class="text-sm text-muted-foreground"><Countdown end={g.valid_before} /></p>
									{#if g.last_command}
										<p class="mt-1 truncate text-sm text-muted-foreground">
											<code>{formatArgv(g.last_command.argv)}</code>
											· {formatAgo(Date.parse(g.last_command.reported_at), clock.now)}
										</p>
									{/if}
								</a>
								<div class="flex shrink-0 gap-1">
									<Button
										size="sm"
										variant="secondary"
										onclick={() => {
											extendFor = g;
											extendOpen = true;
										}}>Extend</Button
									>
									<Button
										size="sm"
										variant="ghost"
										class="text-destructive"
										onclick={() => {
											revokeFor = g;
											revokeOpen = true;
										}}>{g.status === 'offered' ? 'Withdraw' : 'Revoke'}</Button
									>
								</div>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</Section>

		<Section title="Recent activity">
			{#snippet actions()}<a href="/audit" class="text-sm text-muted-foreground">All</a>{/snippet}
			{#if d.recent_audit.length === 0}
				<p class="text-muted-foreground">Nothing yet.</p>
			{:else}
				<ol class="space-y-1.5 text-sm">
					{#each d.recent_audit as ev (ev.id)}
						<li class="flex gap-3">
							<span class="w-20 shrink-0 text-muted-foreground tabular-nums"
								>{formatAgo(Date.parse(ev.ts), clock.now)}</span
							>
							<span class="min-w-0 break-words">
								{actionLabel(ev.action)}
								<span class="text-muted-foreground">· {ev.actor}</span>
								{#if ev.grant_id}<a href="/grants/{ev.grant_id}" class="text-muted-foreground"
										>#{ev.grant_id}</a
									>{/if}
							</span>
						</li>
					{/each}
				</ol>
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
		description="lor ssh stops working right away."
	>
		{#snippet footer()}
			<Button variant="outline" onclick={() => (revokeOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={revoke}
				>{revokeFor?.status === 'offered' ? 'Withdraw' : 'Revoke'}</Button
			>
		{/snippet}
	</Confirm>
{/if}
