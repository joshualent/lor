<script lang="ts">
	// What to show when there's nothing (or something went wrong): an icon,
	// a short title, one sentence, and the next step. "inset" sits inside a
	// list card; "page" stands alone.
	import type { Component, Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';
	import type { Tone } from '#lib/design/variants.js';
	import IconTile from './IconTile.svelte';

	let {
		icon,
		title,
		description,
		tone = 'neutral',
		variant = 'inset',
		size = 'default',
		action,
		children,
		role,
		heading
	}: {
		icon?: Component;
		title: string;
		description?: string;
		tone?: Tone;
		variant?: 'inset' | 'page' | 'plain';
		size?: 'default' | 'compact';
		action?: Snippet;
		children?: Snippet;
		role?: 'status' | 'alert';
		/** Heading level of the title; defaults to h1 for "page", h3 otherwise. */
		heading?: 'h1' | 'h2' | 'h3';
	} = $props();
	const tag = $derived(heading ?? (variant === 'page' ? 'h1' : 'h3'));
</script>

<div
	data-slot="empty-state"
	{role}
	class={cn(
		'flex flex-col items-center px-6 text-center',
		variant === 'inset' && 'rounded-xl bg-card',
		variant === 'inset' && (size === 'compact' ? 'py-5' : 'py-10'),
		variant === 'page' && 'py-16 sm:py-20',
		variant === 'plain' && 'py-8'
	)}
>
	{#if icon}<IconTile
			{icon}
			{tone}
			size={size === 'compact' ? 'md' : 'lg'}
			class={size === 'compact' ? 'mb-2' : 'mb-4'}
		/>{/if}
	<svelte:element
		this={tag}
		class={variant === 'page' ? 'text-headline text-foreground' : 'text-heading text-foreground'}
		>{title}</svelte:element
	>
	{#if description}
		<p class="mt-1 max-w-sm text-callout text-pretty text-muted-foreground">{description}</p>
	{/if}
	{#if children}<div class="mt-3">{@render children()}</div>{/if}
	{#if action}<div class="mt-5 flex flex-wrap justify-center gap-2">{@render action()}</div>{/if}
</div>
