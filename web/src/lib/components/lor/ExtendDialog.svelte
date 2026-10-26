<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { EXTEND_CHOICES, parseDuration } from '#lib/durations.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatClock, formatDuration } from '#lib/format.js';
	import { stepUp } from '#lib/webauthn.js';
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
		<div class="flex flex-wrap gap-2">
			{#each EXTEND_CHOICES as c (c.seconds)}
				<button
					type="button"
					class="h-10 rounded-md border px-3 text-sm {!custom.trim() && seconds === c.seconds
						? 'border-primary bg-primary/10 text-primary'
						: 'border-border hover:bg-secondary'}"
					onclick={() => {
						seconds = c.seconds;
						custom = '';
					}}>{c.label}</button
				>
			{/each}
		</div>
		<Input
			bind:value={custom}
			placeholder="Custom, e.g. 90m or 2d"
			aria-label="Custom extension"
			class="max-w-56"
		/>
		<p class="text-sm text-muted-foreground">
			{#if newEnd}
				Ends {formatClock(Date.parse(end), clock.now)} → would end {formatClock(newEnd, clock.now)}
			{:else}
				Use minutes, hours, days or weeks, like 90m or 2d.
			{/if}
		</p>
	</div>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
		<Button disabled={!chosen || busy} onclick={extend}>
			<KeyRound />
			{busy ? 'Waiting for passkey…' : `Extend by ${chosen ? formatDuration(chosen) : '…'}`}
		</Button>
	{/snippet}
</Confirm>
