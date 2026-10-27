<script lang="ts">
	// A row of filters above a list. Stacks on phones.
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		label,
		children,
		class: className
	}: { label: string; children: Snippet; class?: string } = $props();
</script>

<div
	role="group"
	aria-label={label}
	data-slot="toolbar"
	class={cn('mb-5 flex flex-col gap-3', className)}
>
	{@render children()}
</div>
