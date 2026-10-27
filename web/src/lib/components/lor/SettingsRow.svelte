<script lang="ts">
	// A setting: icon, title, description, and a control on the right
	// (Switch, Button, value). Pass `for` with the control's id so tapping
	// the title toggles it.
	import type { Component, Snippet } from 'svelte';
	import type { Tone } from '#lib/design/variants.js';
	import IconTile from './IconTile.svelte';

	let {
		title,
		description,
		icon,
		tone = 'neutral',
		for: forId,
		control,
		children
	}: {
		title: string;
		description?: string;
		icon?: Component;
		tone?: Tone;
		/** id of the control, to label it. */
		for?: string;
		control?: Snippet;
		/** Extra content under the description. */
		children?: Snippet;
	} = $props();
</script>

<li class="flex min-h-14 items-center gap-3 px-4 py-3" data-slot="settings-row">
	{#if icon}<IconTile {icon} {tone} size="sm" />{/if}
	<div class="min-w-0 flex-1">
		{#if forId}
			<label for={forId} class="block cursor-pointer text-body font-medium">{title}</label>
		{:else}
			<p class="text-body font-medium">{title}</p>
		{/if}
		{#if description}
			<p class="mt-0.5 text-callout text-muted-foreground" id={forId ? `${forId}-desc` : undefined}>
				{description}
			</p>
		{/if}
		{#if children}<div class="mt-2">{@render children()}</div>{/if}
	</div>
	{#if control}<div class="flex shrink-0 items-center gap-1">{@render control()}</div>{/if}
</li>
