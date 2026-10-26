<script lang="ts">
	import { DURATION_CHOICES, isLong, parseDuration } from '#lib/durations.js';
	import { formatDuration } from '#lib/format.js';
	import { Input } from '#lib/components/ui/input/index.js';

	let {
		value = $bindable(),
		max,
		id = 'duration'
	}: { value: number; max?: number; id?: string } = $props();

	let custom = $state(false);
	let customText = $state('');
	const presets = $derived(DURATION_CHOICES.filter((c) => !max || c.seconds <= max));
	const isPreset = $derived(presets.some((c) => c.seconds === value));

	$effect(() => {
		if (!isPreset && !custom) {
			custom = true;
			customText = formatDuration(value).replace(/\s/g, '');
		}
	});

	function choose(s: number) {
		custom = false;
		value = s;
	}

	function onCustom(e: Event) {
		customText = (e.currentTarget as HTMLInputElement).value;
		const s = parseDuration(customText);
		if (s) value = s;
	}

	const customInvalid = $derived(
		custom && customText.trim() !== '' && parseDuration(customText) === null
	);
</script>

<div class="space-y-2">
	<div class="flex flex-wrap gap-2" role="radiogroup" aria-labelledby="{id}-label">
		{#each presets as c (c.seconds)}
			<button
				type="button"
				role="radio"
				aria-checked={!custom && value === c.seconds}
				class="h-9 rounded-md border px-3 text-sm transition-colors {!custom && value === c.seconds
					? 'border-primary bg-primary/10 text-primary'
					: 'border-border hover:bg-secondary'}"
				onclick={() => choose(c.seconds)}>{c.label}</button
			>
		{/each}
		<button
			type="button"
			role="radio"
			aria-checked={custom}
			class="h-9 rounded-md border px-3 text-sm {custom
				? 'border-primary bg-primary/10 text-primary'
				: 'border-border hover:bg-secondary'}"
			onclick={() => {
				custom = true;
				customText = '';
			}}>Custom</button
		>
	</div>
	{#if custom}
		<Input
			{id}
			value={customText}
			oninput={onCustom}
			placeholder="e.g. 90m, 2h, 3d"
			aria-invalid={customInvalid}
			aria-describedby="{id}-hint"
			class="max-w-48"
			autocomplete="off"
		/>
		<p id="{id}-hint" class="text-sm text-muted-foreground">
			{customInvalid ? 'Use minutes, hours, days or weeks, like 90m or 2d.' : formatDuration(value)}
		</p>
	{/if}
	{#if isLong(value)}
		<p class="text-sm text-warning" data-testid="long-note">
			That's longer than a day. Make sure that's what you want.
		</p>
	{/if}
</div>
