<script lang="ts">
	// A background refresh failed but the last data is still shown: say so,
	// say how old it is, and offer a retry. (A failed first load uses
	// ErrorState instead.)
	import CloudOff from '@lucide/svelte/icons/cloud-off';
	import RotateCw from '@lucide/svelte/icons/rotate-cw';
	import { errorMessage } from '#lib/errors.js';
	import { formatTimeOfDay } from '#lib/format.js';
	import { Button } from '#lib/components/ui/button/index.js';

	let {
		error,
		updatedAt,
		onretry
	}: {
		error: unknown;
		/** ms since epoch of the shown data */ updatedAt: number;
		onretry: () => void;
	} = $props();
</script>

<div
	role="status"
	data-slot="stale-notice"
	class="mb-5 flex items-center gap-3 rounded-xl border border-warning-edge bg-warning-tint px-3.5 py-2.5"
>
	<CloudOff class="size-4 shrink-0 text-warning" aria-hidden="true" />
	<p class="min-w-0 flex-1 text-callout">
		<span class="font-medium text-warning">Couldn't refresh.</span>
		Showing data from {formatTimeOfDay(updatedAt)}.
		<span class="text-muted-foreground">{errorMessage(error)}</span>
	</p>
	<Button size="sm" variant="secondary" onclick={onretry}><RotateCw /> Retry</Button>
</div>
