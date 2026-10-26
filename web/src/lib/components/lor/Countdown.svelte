<script lang="ts">
	import { clock } from '#lib/clock.svelte.js';
	import { formatCountdown } from '#lib/format.js';

	let { end, class: className = '' }: { end: string | undefined; class?: string } = $props();
	const text = $derived(end ? formatCountdown(Date.parse(end), clock.now) : '');
	const soon = $derived(end ? Date.parse(end) - clock.now < 5 * 60_000 : false);
</script>

{#if end}
	<span
		class="tabular-nums {soon ? 'text-warning' : ''} {className}"
		title={new Date(end).toLocaleString()}>{text === 'ended' ? 'ended' : `${text} left`}</span
	>
{/if}
