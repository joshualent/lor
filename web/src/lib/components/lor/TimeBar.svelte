<script lang="ts">
	// How much of a grant's window is left, as a thin bar. Decorative: the
	// countdown next to it carries the information.
	import { clock } from '#lib/clock.svelte.js';
	import { remainingFraction, remainingTone } from '#lib/status.js';
	import { cn } from '#lib/utils.js';

	let {
		start,
		end,
		offer = false,
		class: className
	}: {
		start: string;
		end: string;
		/** An unclaimed offer (info tone). */ offer?: boolean;
		class?: string;
	} = $props();
	const startMs = $derived(Date.parse(start));
	const endMs = $derived(Date.parse(end));
	const left = $derived(remainingFraction(startMs, endMs, clock.now));
	const tone = $derived(remainingTone(endMs, clock.now));
	const fill = $derived(
		tone === 'danger'
			? 'bg-destructive'
			: tone === 'warning'
				? 'bg-warning'
				: offer
					? 'bg-info'
					: 'bg-success'
	);
</script>

<div
	class={cn('h-1 overflow-hidden rounded-full bg-secondary', className)}
	aria-hidden="true"
	data-slot="time-bar"
>
	<div
		class="h-full rounded-full transition-[width] duration-1000 ease-linear {fill}"
		style="width: {(left * 100).toFixed(2)}%"
	></div>
</div>
