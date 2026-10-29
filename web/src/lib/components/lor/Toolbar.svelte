<script lang="ts">
	// A row of filters above a list. Stacks on phones.
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		id,
		label,
		children,
		class: className
	}: { id?: string; label: string; children: Snippet; class?: string } = $props();
</script>

<div
	{id}
	tabindex="-1"
	role="group"
	aria-label={label}
	data-slot="toolbar"
	class={cn('mb-5 flex flex-col gap-3', className)}
>
	{@render children()}
</div>
