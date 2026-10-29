<script lang="ts">
	// The page's decision buttons in one row: put the secondary action first
	// and give the primary one `flex-1`. On phones the bar sticks to the
	// bottom of the screen; compact windows use the flow to preserve reading space.
	import type { Snippet } from 'svelte';

	let { children, note, id }: { children: Snippet; note?: Snippet; id?: string } = $props();
</script>

<div
	{id}
	tabindex="-1"
	role="group"
	aria-label="Decision"
	data-slot="action-bar"
	class="decision-actions sticky bottom-0 z-10 -mx-4 space-y-2.5 border-t border-border bg-background/90 px-4 pt-3 pb-[max(0.875rem,env(safe-area-inset-bottom))] backdrop-blur-md sm:mx-0 sm:rounded-xl sm:border sm:pb-3"
>
	{#if note}{@render note()}{/if}
	<div class="flex items-stretch gap-2">{@render children()}</div>
</div>

<style>
	@media (max-width: 359px), (max-height: 699px) {
		.decision-actions {
			position: static;
		}
	}
</style>
