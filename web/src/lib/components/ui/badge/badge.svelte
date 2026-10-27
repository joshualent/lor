<script lang="ts">
	// A short status or category label. Badge only what needs noticing
	// (see docs/design-system.md); plain metadata stays plain text.
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '#lib/utils.js';
	import { badgeVariants, type Tone } from '#lib/design/variants.js';

	let {
		tone = 'neutral',
		size = 'sm',
		class: className,
		children,
		...rest
	}: HTMLAttributes<HTMLSpanElement> & {
		tone?: Tone;
		size?: 'sm' | 'md';
		children: Snippet;
	} = $props();
</script>

<span data-slot="badge" data-tone={tone} class={cn(badgeVariants({ tone, size }), className)} {...rest}
	>{@render children()}</span
>
