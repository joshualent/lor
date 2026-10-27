<script lang="ts">
	// Label/value pairs on a grouped-list surface (detail pages). Children are
	// DescriptionItem.
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		class: className,
		children,
		...rest
	}: { class?: string; children: Snippet; [key: `data-${string}`]: unknown } = $props();
</script>

<dl
	data-slot="description-list"
	class={cn('divide-y divide-separator overflow-hidden rounded-xl bg-card', className)}
	{...rest}
>
	{@render children()}
</dl>
