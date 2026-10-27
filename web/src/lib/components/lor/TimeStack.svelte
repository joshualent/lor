<script lang="ts">
	// A labelled point in time on deliberate lines: label, time (AM/PM kept
	// together) with its zone, and the day. For before/after comparisons.
	import { clock } from '#lib/clock.svelte.js';
	import { clockParts } from '#lib/format.js';
	import { cn } from '#lib/utils.js';

	let {
		label,
		ms,
		accent = false,
		note
	}: {
		label: string;
		ms: number;
		/** The new/proposed value (accent color). */
		accent?: boolean;
		/** Extra line, e.g. "12 min left" or "+1 h". */
		note?: string;
	} = $props();
	const p = $derived(clockParts(ms, clock.now));
</script>

<div class="min-w-0" data-slot="time-stack">
	<p class="text-footnote text-muted-foreground">{label}</p>
	<p class={cn('text-headline whitespace-nowrap nums', accent && 'text-primary')}>
		{p.time}<span class="ml-1 text-footnote font-normal text-muted-foreground">{p.zone}</span>
	</p>
	<p class="text-footnote text-muted-foreground">{p.day}{note ? ` · ${note}` : ''}</p>
</div>
