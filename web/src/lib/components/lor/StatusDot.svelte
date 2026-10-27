<script lang="ts">
	// A colored dot for state at a glance. Always pair it with text, or pass
	// `label` so screen readers get the meaning.
	import { cn } from '#lib/utils.js';
	import { statusDotVariants, type Tone } from '#lib/design/variants.js';

	let {
		tone = 'neutral',
		pulse = false,
		label,
		class: className
	}: { tone?: Tone; pulse?: boolean; label?: string; class?: string } = $props();
</script>

<span
	data-slot="status-dot"
	class={cn(statusDotVariants({ tone, pulse }), className)}
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
></span>
