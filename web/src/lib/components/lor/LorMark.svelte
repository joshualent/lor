<script lang="ts">
	// Lor's mark (the same glyph as the favicon), inline so it needs no request.
	import { cn } from '#lib/utils.js';

	let { class: className }: { class?: string } = $props();
</script>

<svg viewBox="0 0 32 32" class={cn('size-14', className)} aria-hidden="true" data-slot="lor-mark">
	<rect width="32" height="32" rx="8" class="fill-card" />
	<path
		d="M11 8v16h10"
		fill="none"
		class="stroke-primary"
		stroke-width="3"
		stroke-linecap="round"
		stroke-linejoin="round"
	/>
</svg>
