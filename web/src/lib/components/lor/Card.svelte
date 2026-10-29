<script lang="ts">
	// A painted surface for free-form content (forms, summaries). For rows of
	// things use List; for label/value pairs use DescriptionList.
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		padding = 'md',
		class: className,
		children,
		...rest
	}: {
		padding?: 'none' | 'md' | 'lg';
		class?: string;
		children: Snippet;
		[key: `data-${string}`]: unknown;
	} = $props();
</script>

<div
	data-slot="card"
	class={cn(
		'rounded-xl border border-border bg-card',
		padding === 'md' && 'p-4',
		padding === 'lg' && 'p-4 sm:p-5',
		className
	)}
	{...rest}
>
	{@render children()}
</div>
