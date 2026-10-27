<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import ArrowRight from '@lucide/svelte/icons/arrow-right';
	import TimerReset from '@lucide/svelte/icons/timer-reset';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatCountdown, formatDuration } from '#lib/format.js';
	import { keys, fetchExtension, LIVE_MS } from '#lib/queries.js';
	import { passkeyName } from '#lib/status.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import ActionBar from '#lib/components/lor/ActionBar.svelte';
	import Card from '#lib/components/lor/Card.svelte';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import DescriptionItem from '#lib/components/lor/DescriptionItem.svelte';
	import DescriptionList from '#lib/components/lor/DescriptionList.svelte';
	import DurationPicker from '#lib/components/lor/DurationPicker.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import LevelBadge from '#lib/components/lor/LevelBadge.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import PageSkeleton from '#lib/components/lor/PageSkeleton.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import StatusBadge from '#lib/components/lor/StatusBadge.svelte';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';
	import TimeStack from '#lib/components/lor/TimeStack.svelte';
	import UnverifiedText from '#lib/components/lor/UnverifiedText.svelte';
	import Warnings from '#lib/components/lor/Warnings.svelte';

	const id = $derived(Number(page.params.id));
	const qc = useQueryClient();
	const q = createQuery(() => ({
		queryKey: keys.extension(id),
		queryFn: () => fetchExtension(id),
		refetchInterval: LIVE_MS
	}));
	const d = $derived(q.data);

	let duration = $state(3600);
	let durationValid = $state(true);
	let initialized = $state(false);
	$effect(() => {
		if (d && !initialized) {
			duration = d.extension.requested_duration_s;
			initialized = true;
		}
	});

	let busy = $state(false);
	let denyOpen = $state(false);
	let note = $state('');

	function refresh() {
		qc.invalidateQueries({ queryKey: ['extension'] });
		qc.invalidateQueries({ queryKey: keys.grants });
		qc.invalidateQueries({ queryKey: keys.dashboard });
	}

	async function approve() {
		if (!d) return;
		busy = true;
		try {
			await stepUp({
				kind: 'approve_extension',
				extension_id: d.extension.id,
				duration_s: duration
			});
			toast.success(`Extended ${d.grant.host.name} by ${formatDuration(duration)}`);
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
			refresh();
		}
	}

	async function deny() {
		if (!d) return;
		busy = true;
		try {
			await call(
				api.POST('/extensions/{id}/deny', {
					params: { path: { id: d.extension.id } },
					body: { note }
				})
			);
			toast.success('Extension denied');
			denyOpen = false;
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
			refresh();
		}
	}

	const end = $derived(d?.current_end ? Date.parse(d.current_end) : 0);
	const would = $derived(end + duration * 1000);
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
{:else if d}
	{@const g = d.grant}
	{#if q.isError && q.data}<StaleNotice
			error={q.error}
			updatedAt={q.dataUpdatedAt}
			fetching={q.isFetching}
			onretry={() => q.refetch()}
		/>{/if}
	<article class="space-y-7" data-testid="extension">
		<PageHeader
			title="{g.agent.name} wants more time on {g.host.name}"
			back={{ href: '/', label: 'Home' }}
		>
			{#snippet leading()}<IconTile icon={TimerReset} tone="warning" size="lg" />{/snippet}
			{#snippet heading()}
				<span class="block text-callout font-normal tracking-normal text-muted-foreground"
					>{g.agent.name} wants more time on</span
				>{g.host.name}
			{/snippet}
			{#snippet meta()}
				<StatusBadge status={d.extension.status} kind="extension" />
				<LevelBadge level={g.level} />
				<span class="text-footnote text-muted-foreground">
					Extension #{d.extension.id} of <a href="/grants/{g.id}">grant #{g.id}</a> ·
					{formatAgo(Date.parse(d.extension.requested_at), clock.now)}
				</span>
			{/snippet}
		</PageHeader>

		{#if d.actions.approve}
			<Warnings warnings={g.warnings} level={g.level} />

			<UnverifiedText text={d.extension.reason} />

			<Section title="Time">
				<Card>
					<div
						class="grid grid-cols-[1fr_auto_1fr] items-center gap-x-3"
						data-testid="extension-times"
					>
						<TimeStack label="Now ends" ms={end} note="{formatCountdown(end, clock.now)} left" />
						<ArrowRight class="size-5 text-icon-muted" aria-label="would become" />
						{#if durationValid}
							<TimeStack label="If approved" ms={would} accent note="+{formatDuration(duration)}" />
						{:else}
							<p class="text-callout text-muted-foreground">Enter a duration</p>
						{/if}
					</div>
					<div class="mt-4 border-t border-separator pt-4">
						<p class="mb-3 text-callout text-muted-foreground">
							Add (the level stays {g.level === 'sudo' ? 'sudo' : 'standard user'})
						</p>
						<DurationPicker
							bind:value={duration}
							bind:valid={durationValid}
							id="ext-duration"
							label="Add"
						/>
					</div>
				</Card>
			</Section>

			<ActionBar>
				<Button size="lg" variant="outline" disabled={busy} onclick={() => (denyOpen = true)}
					>Deny</Button
				>
				<Button
					size="lg"
					class="h-auto min-h-12 flex-1 py-2 leading-snug whitespace-normal"
					disabled={busy || !durationValid}
					onclick={approve}
					data-testid="approve-extension"
				>
					{#if busy}<Spinner />{:else}<KeyRound />{/if}
					{busy
						? 'Waiting for passkey…'
						: durationValid
							? `Extend ${g.host.name} by ${formatDuration(duration)}`
							: `Extend ${g.host.name}: enter a duration`}
				</Button>
			</ActionBar>
		{:else}
			<Section title="Outcome">
				<DescriptionList>
					<DescriptionItem label="Asked for"
						>+{formatDuration(d.extension.requested_duration_s)}</DescriptionItem
					>
					{#if d.extension.approved_duration_s}<DescriptionItem label="Approved"
							>+{formatDuration(d.extension.approved_duration_s)}</DescriptionItem
						>{/if}
					{#if d.extension.decided_by}<DescriptionItem label="Decided by"
							>{passkeyName(d.extension.decided_by)}</DescriptionItem
						>{/if}
					{#if d.extension.decision_note}<DescriptionItem label="Note" layout="stacked"
							>{d.extension.decision_note}</DescriptionItem
						>{/if}
					<DescriptionItem label="Grant">
						<a href="/grants/{g.id}" class="inline-flex items-center gap-2"
							>#{g.id} <StatusBadge status={g.status} /></a
						>
					</DescriptionItem>
				</DescriptionList>
			</Section>
			<UnverifiedText text={d.extension.reason} />
		{/if}
	</article>

	<Confirm
		bind:open={denyOpen}
		title="Deny this extension?"
		description="The current grant keeps its end time."
	>
		<div class="space-y-2">
			<Label for="ext-note">Note (optional)</Label>
			<Textarea id="ext-note" bind:value={note} maxlength={1000} rows={3} />
		</div>
		{#snippet footer()}
			<Button variant="outline" onclick={() => (denyOpen = false)}>Cancel</Button>
			<Button variant="destructive" disabled={busy} onclick={deny}>Deny</Button>
		{/snippet}
	</Confirm>
{/if}
