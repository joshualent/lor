<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import ArrowRight from '@lucide/svelte/icons/arrow-right';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { EXTEND_CHOICES, parseDuration } from '#lib/durations.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatClock, formatDuration } from '#lib/format.js';
	import { stepUp } from '#lib/webauthn.js';
	import ChipGroup from './ChipGroup.svelte';
	import Confirm from './Confirm.svelte';

	let {
		open = $bindable(false),
		grantId,
		host,
		end,
		onDone
	}: { open: boolean; grantId: number; host: string; end: string; onDone: () => void } = $props();

	let seconds = $state(3600);
	let custom = $state('');
	let busy = $state(false);

	const chosen = $derived(custom.trim() ? parseDuration(custom) : seconds);
	// Added time counts from the current end, not from now.
	const newEnd = $derived(chosen ? Date.parse(end) + chosen * 1000 : null);
	const options = $derived(
		EXTEND_CHOICES.map((c) => ({
			value: c.seconds,
			label: c.label,
			selected: !custom.trim() && seconds === c.seconds
		}))
	);

	async function extend() {
		if (!chosen) return;
		busy = true;
		try {
			await stepUp({ kind: 'extend_grant', grant_id: grantId, duration_s: chosen });
			toast.success(`Extended ${host} by ${formatDuration(chosen)}`);
			open = false;
			custom = '';
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
		<ChipGroup
			{options}
			value={seconds}
			label="Add time"
			onchange={(s) => {
				seconds = s;
				custom = '';
			}}
		/>
		<Input
			bind:value={custom}
			placeholder="Custom, e.g. 90m or 2d"
			aria-label="Custom extension"
			class="max-w-56"
		/>
		<div class="flex items-center gap-2 rounded-lg bg-card px-3 py-2.5 text-callout">
			{#if newEnd}
				<span class="text-muted-foreground">Ends {formatClock(Date.parse(end), clock.now)}</span>
				<ArrowRight class="size-4 text-icon-muted" aria-hidden="true" />
				<span class="font-medium text-primary">{formatClock(newEnd, clock.now)}</span>
			{:else}
				<span class="text-muted-foreground">Use minutes, hours, days or weeks, like 90m or 2d.</span
				>
			{/if}
		</div>
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
		<Button disabled={!chosen || busy} onclick={extend}>
			{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : `Extend by ${chosen ? formatDuration(chosen) : '…'}`}
		</Button>
	{/snippet}
</Confirm>
