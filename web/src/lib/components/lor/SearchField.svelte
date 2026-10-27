<script lang="ts">
	// A search/filter input with a leading icon and a clear button.
	import Search from '@lucide/svelte/icons/search';
	import X from '@lucide/svelte/icons/x';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { Input } from '#lib/components/ui/input/index.js';
	import { cn } from '#lib/utils.js';

	let {
		value = $bindable(''),
		label,
		class: className,
		...rest
	}: Omit<HTMLInputAttributes, 'value' | 'type' | 'files'> & {
		value?: string;
		label: string;
	} = $props();
</script>

<div class={cn('relative', className)} data-slot="search-field">
	<Search
		class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-icon-muted"
		aria-hidden="true"
	/>
	<Input
		type="search"
		bind:value
		aria-label={label}
		autocomplete="off"
		class="pr-9 pl-9 [&::-webkit-search-cancel-button]:hidden"
		{...rest}
	/>
	{#if value}
		<button
			type="button"
			class="absolute top-1/2 right-1.5 grid size-7 -translate-y-1/2 place-items-center rounded-md text-muted-foreground focus-ring after:absolute after:-inset-2 after:content-[''] hover:text-foreground"
			aria-label="Clear {label}"
			onclick={() => (value = '')}
		>
			<X class="size-4" aria-hidden="true" />
		</button>
	{/if}
</div>
