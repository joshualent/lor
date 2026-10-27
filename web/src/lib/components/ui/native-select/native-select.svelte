<script lang="ts">
	// A styled native <select>: the OS picker is the best one on phones.
	import ChevronDown from '@lucide/svelte/icons/chevrons-up-down';
	import type { HTMLSelectAttributes } from 'svelte/elements';
	import { cn, type WithElementRef } from '#lib/utils.js';
	import { useFieldAria } from '#lib/design/field-context.js';

	let {
		ref = $bindable(null),
		value = $bindable(),
		class: className,
		children,
		...restProps
	}: WithElementRef<HTMLSelectAttributes, HTMLSelectElement> = $props();
	const fieldAria = useFieldAria();
	const aria = $derived(fieldAria(restProps.id));
</script>

<div class={cn('relative w-full', className)} data-slot="native-select-wrapper">
	<select
		bind:this={ref}
		bind:value
		data-slot="native-select"
		class="h-10 w-full min-w-0 cursor-pointer appearance-none rounded-md border border-input bg-input/30 py-1 pr-9 pl-3 text-base transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive/50 aria-invalid:ring-3 aria-invalid:ring-destructive/40 md:text-sm"
		{...aria}
		{...restProps}
	>
		{@render children?.()}
	</select>
	<ChevronDown
		class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-icon-muted"
		aria-hidden="true"
	/>
</div>
