<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import ArrowRight from '@lucide/svelte/icons/arrow-right';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { EXTEND_CHOICES } from '#lib/durations.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatClock, formatDuration } from '#lib/format.js';
	import { stepUp } from '#lib/webauthn.js';
	import DurationPicker from './DurationPicker.svelte';
	import Confirm from './Confirm.svelte';

	let {
		open = $bindable(false),
		grantId,
		host,
		end,
		onDone
	}: { open: boolean; grantId: number; host: string; end: string; onDone: () => void } = $props();

	let seconds = $state(3600);
	let valid = $state(true);
	let busy = $state(false);

	const chosen = $derived(valid ? seconds : null);
	// Added time counts from the current end, not from now.
	const newEnd = $derived(chosen ? Date.parse(end) + chosen * 1000 : null);

	async function extend() {
		if (!chosen) return;
		busy = true;
		try {
			await stepUp({ kind: 'extend_grant', grant_id: grantId, duration_s: chosen });
			toast.success(`Extended ${host} by ${formatDuration(chosen)}`);
			open = false;
			onDone();
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}
</script>

<Confirm
	bind:open
	title="Extend access to {host}"
	description="Added time counts from the current end."
>
	<div class="space-y-4">
		<DurationPicker
			bind:value={seconds}
			bind:valid
			choices={EXTEND_CHOICES}
			id="extend-duration"
			label="Add time"
			placeholder="e.g. 90m or 2d"
		/>
		{#if newEnd}
			<div
				class="grid grid-cols-[1fr_auto_1fr] items-center gap-x-3 rounded-lg bg-card px-3.5 py-3"
			>
				<div>
					<p class="text-footnote text-muted-foreground">Current end</p>
					<p class="font-medium nums">{formatClock(Date.parse(end), clock.now)}</p>
				</div>
				<ArrowRight class="size-4 text-icon-muted" aria-label="becomes" />
				<div>
					<p class="text-footnote text-muted-foreground">New end</p>
					<p class="font-medium text-primary nums">{formatClock(newEnd, clock.now)}</p>
				</div>
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
		<Button disabled={!chosen || busy} onclick={extend}>
			{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : `Extend by ${chosen ? formatDuration(chosen) : '…'}`}
		</Button>
	{/snippet}
</Confirm>
