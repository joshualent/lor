<script lang="ts">
	// A message that needs reading: warnings, risks, explanations. Status is
	// carried by the title text as well as the color and icon.
	import type { Component, Snippet } from 'svelte';
	import Info from '@lucide/svelte/icons/info';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import OctagonAlert from '@lucide/svelte/icons/octagon-alert';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import { cn } from '#lib/utils.js';
	import { calloutVariants, type Tone } from '#lib/design/variants.js';

	let {
		tone = 'neutral',
		icon,
		title,
		children,
		action,
		class: className,
		...rest
	}: {
		tone?: Tone;
		icon?: Component;
		title?: string;
		children?: Snippet;
		action?: Snippet;
		class?: string;
		[key: `data-${string}`]: unknown;
	} = $props();

	const DEFAULT_ICON: Partial<Record<Tone, Component>> = {
		danger: OctagonAlert,
		warning: TriangleAlert,
		success: CircleCheck
	};
	const Icon = $derived(icon ?? DEFAULT_ICON[tone] ?? Info);
	const s = $derived(calloutVariants({ tone }));
</script>

<div data-slot="callout" data-tone={tone} class={cn(s.root(), className)} {...rest}>
	<Icon class={s.icon()} aria-hidden="true" />
	<div class="min-w-0 flex-1">
		{#if title}<p class={s.title()}>{title}</p>{/if}
		{#if children}<div class={title ? s.body() : 'text-callout'}>{@render children()}</div>{/if}
		{#if action}<div class="mt-3 flex flex-wrap gap-2">{@render action()}</div>{/if}
	</div>
</div>
