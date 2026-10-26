<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { page } from '$app/state';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { formatAgo, formatClock, formatCountdown, formatDuration } from '#lib/format.js';
	import { keys, fetchExtension, LIVE_MS } from '#lib/queries.js';
	import { stepUp } from '#lib/webauthn.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import DurationPicker from '#lib/components/lor/DurationPicker.svelte';
	import Row from '#lib/components/lor/Row.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import Status from '#lib/components/lor/Status.svelte';
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
	<p class="text-muted-foreground">Loading…</p>
{:else if q.isError}
	<p class="text-destructive">{errorMessage(q.error)}</p>
{:else if d}
	{@const g = d.grant}
	<article class="space-y-6">
		<header>
			<h1 class="text-2xl leading-tight">
				<span class="font-normal text-muted-foreground">{g.agent.name} wants more time on</span>
				{g.host.name}
			</h1>
			<p class="mt-1 text-sm text-muted-foreground">
				Extension #{d.extension.id} of <a href="/grants/{g.id}">grant #{g.id}</a> ·
				{formatAgo(Date.parse(d.extension.requested_at), clock.now)}
			</p>
		</header>

		{#if d.actions.approve}
			<Warnings warnings={g.warnings} level={g.level} />

			<Section>
				<dl>
					<Row label="Level"
						>{g.level === 'sudo' ? 'sudo (root), unchanged' : 'standard user, unchanged'}</Row
					>
					<Row label="Now"
						>ends {formatClock(end, clock.now)} ({formatCountdown(end, clock.now)} left)</Row
					>
					<Row label="If approved"
						><span class="text-primary">would end {formatClock(would, clock.now)}</span></Row
					>
				</dl>
			</Section>

			<Section title="Add">
				<DurationPicker bind:value={duration} id="ext-duration" />
			</Section>

			<UnverifiedText text={d.extension.reason} />

			<div
				class="sticky bottom-0 -mx-4 flex flex-col gap-2 border-t bg-background/95 px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))] backdrop-blur sm:static sm:mx-0 sm:flex-row sm:border-0 sm:px-0"
			>
				<Button
					size="lg"
					class="sm:flex-1"
					disabled={busy}
					onclick={approve}
					data-testid="approve-extension"
				>
					<KeyRound />
					{busy ? 'Waiting for passkey…' : `Extend ${g.host.name} by ${formatDuration(duration)}`}
				</Button>
				<Button size="lg" variant="outline" disabled={busy} onclick={() => (denyOpen = true)}
					>Deny</Button
				>
			</div>
		{:else}
			<Section>
				<dl>
					<Row label="Status"><Status status={d.extension.status} /></Row>
					<Row label="Asked for">+{formatDuration(d.extension.requested_duration_s)}</Row>
					{#if d.extension.approved_duration_s}<Row label="Approved"
							>+{formatDuration(d.extension.approved_duration_s)}</Row
						>{/if}
					{#if d.extension.decision_note}<Row label="Note">{d.extension.decision_note}</Row>{/if}
					<Row label="Grant"
						><a href="/grants/{g.id}">#{g.id}</a> · <Status status={g.status} /></Row
					>
				</dl>
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
