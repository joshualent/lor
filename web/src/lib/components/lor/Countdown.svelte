<script lang="ts">
	// Time left until `end`, ticking locally. Turns warning in the last five
	// minutes and danger in the last minute. Not a live region (it would
	// announce every second); the full end time is in the title.
	import Timer from '@lucide/svelte/icons/timer';
	import { clock } from '#lib/clock.svelte.js';
	import { formatCountdown } from '#lib/format.js';
	import { remainingTone } from '#lib/status.js';
	import { toneText } from '#lib/design/variants.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { cn } from '#lib/utils.js';

	let {
		end,
		variant = 'text',
		suffix = 'left',
		class: className = ''
	}: {
		end: string | undefined;
		variant?: 'text' | 'badge';
		suffix?: string;
		class?: string;
	} = $props();

	const endMs = $derived(end ? Date.parse(end) : 0);
	const text = $derived(end ? formatCountdown(endMs, clock.now) : '');
	const tone = $derived(remainingTone(endMs, clock.now));
	const label = $derived(text === 'ended' ? 'ended' : suffix ? `${text} ${suffix}` : text);
</script>

{#if end}
	{#if variant === 'badge'}
		<Badge {tone} title={new Date(end).toLocaleString()} class="nums" data-tone={tone}>
			<Timer aria-hidden="true" />{label}
		</Badge>
	{:else}
		<span
			class={cn('nums', tone !== 'neutral' && toneText[tone], className)}
			data-tone={tone}
			title={new Date(end).toLocaleString()}>{label}</span
		>
	{/if}
{/if}
