<script lang="ts">
	// A grouped ("inset") list: rounded card surface, hairline dividers.
	// Children are ListRow (or other <li>) elements.
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';
	import { listVariants } from '#lib/design/variants.js';

	let {
		label,
		ordered = false,
		divided = true,
		class: className,
		children,
		...rest
	}: {
		/** Accessible name when no visible heading labels the list. */
		label?: string;
		/** <ol> for sequences (logs, timelines). */
		ordered?: boolean;
		divided?: boolean;
		class?: string;
		children: Snippet;
		[key: `data-${string}`]: unknown;
	} = $props();
</script>

<svelte:element
	this={ordered ? 'ol' : 'ul'}
	role="list"
	aria-label={label}
	data-slot="list"
	class={cn(listVariants({ divided }), className)}
	{...rest}
>
	{@render children()}
</svelte:element>
