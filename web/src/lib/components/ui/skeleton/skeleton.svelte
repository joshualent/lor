<script lang="ts">
	// Placeholder while content loads. Decorative: wrap groups of skeletons
	// in an element with aria-busy and a visually hidden "Loading…" label.
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '#lib/utils.js';
	import { skeletonVariants } from '#lib/design/variants.js';

	let {
		shape = 'line',
		class: className,
		...rest
	}: HTMLAttributes<HTMLDivElement> & { shape?: 'line' | 'block' | 'circle' | 'tile' } = $props();
</script>

<div
	data-slot="skeleton"
	aria-hidden="true"
	class={cn(skeletonVariants({ shape }), className)}
	{...rest}
></div>
