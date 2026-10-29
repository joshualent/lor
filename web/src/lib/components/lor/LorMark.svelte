<script lang="ts">
	// Share the approved artwork and its cache version with the favicon in app.html.
	import { asset } from '$app/paths';
	import { cn } from '#lib/utils.js';

	let { class: className }: { class?: string } = $props();
</script>

<img
	src={`${asset('favicon.svg')}?v=e11f4261`}
	alt=""
	width="56"
	height="56"
	class={cn('size-14', className)}
	aria-hidden="true"
	data-slot="lor-mark"
/>
