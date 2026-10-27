<script lang="ts">
	// On/off control for settings (bits-ui Switch: role="switch", Space toggles).
	import { Switch as SwitchPrimitive } from 'bits-ui';
	import { cn, type WithoutChildrenOrChild } from '#lib/utils.js';

	let {
		ref = $bindable(null),
		checked = $bindable(false),
		class: className,
		...restProps
	}: WithoutChildrenOrChild<SwitchPrimitive.RootProps> = $props();
</script>

<SwitchPrimitive.Root
	bind:ref
	bind:checked
	data-slot="switch"
	class={cn(
		'peer focus-ring relative inline-flex h-6 w-10 shrink-0 cursor-pointer items-center rounded-full border border-transparent transition-colors duration-(--duration-fast) disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input after:absolute after:-inset-2',
		className
	)}
	{...restProps}
>
	<SwitchPrimitive.Thumb
		data-slot="switch-thumb"
		class="pointer-events-none block size-5 rounded-full bg-foreground shadow-sm transition-transform duration-(--duration-medium) ease-(--ease-standard) data-[state=checked]:translate-x-[1.0625rem] data-[state=checked]:bg-primary-foreground data-[state=unchecked]:translate-x-0.5"
	/>
</SwitchPrimitive.Root>
