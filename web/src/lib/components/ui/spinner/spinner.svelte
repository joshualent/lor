<script lang="ts">
	// Indeterminate progress (e.g. while a passkey prompt is open). Pair it
	// with visible text; the icon itself is decorative.
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import { cn } from '#lib/utils.js';

	let { class: className }: { class?: string } = $props();
</script>

<LoaderCircle
	data-slot="spinner"
	aria-hidden="true"
	class={cn('animate-spin motion-reduce:animate-none', className)}
/>
