<script lang="ts">
	// A small tinted square with an icon (or a monogram) that says what a row
	// is at a glance. The tone carries meaning; see docs/design-system.md.
	import type { Component } from 'svelte';
	import { cn } from '#lib/utils.js';
	import { iconTileVariants, type Tone } from '#lib/design/variants.js';

	let {
		icon: Icon,
		text,
		tone = 'neutral',
		size = 'md',
		label,
		class: className
	}: {
		icon?: Component;
		/** Monogram instead of an icon (first letter is used). */
		text?: string;
		tone?: Tone;
		size?: 'sm' | 'md' | 'lg';
		/** Accessible label; without it the tile is decorative. */
		label?: string;
		class?: string;
	} = $props();
</script>

<span
	data-slot="icon-tile"
	data-tone={tone}
	class={cn(iconTileVariants({ tone, size }), className)}
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	{#if Icon}<Icon />{:else if text}{text.slice(0, 1).toUpperCase()}{/if}
</span>
