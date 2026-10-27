<script lang="ts">
	// The page's decision buttons. On phones it sticks to the bottom of the
	// screen (reachable one-handed); on wider screens it sits in the flow.
	import type { Snippet } from 'svelte';

	let { children, note }: { children: Snippet; note?: Snippet } = $props();
</script>

<div
	data-slot="action-bar"
	class="sticky bottom-0 z-10 -mx-4 space-y-3 border-t border-border bg-background/90 px-4 pt-3 pb-[max(0.875rem,env(safe-area-inset-bottom))] backdrop-blur-md sm:static sm:mx-0 sm:border-0 sm:bg-transparent sm:px-0 sm:pt-0 sm:backdrop-blur-none"
>
	{#if note}{@render note()}{/if}
	<div class="flex flex-col gap-2 sm:flex-row">{@render children()}</div>
</div>
