<script lang="ts">
	import { DURATION_CHOICES, isLong, parseDuration } from '#lib/durations.js';
	import { formatDuration } from '#lib/format.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import ChipGroup from './ChipGroup.svelte';

	let {
		value = $bindable(),
		// Write-only bindable prop (the parent reads it); ESLint can't see that.
		// eslint-disable-next-line no-useless-assignment
		valid = $bindable(),
		max,
		id = 'duration',
		label = 'Duration'
	}: { value: number; valid?: boolean; max?: number; id?: string; label?: string } = $props();

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

	function onCustom(e: Event) {
		customText = (e.currentTarget as HTMLInputElement).value;
		const s = parseDuration(customText);
		if (s) value = s;
	}

	// Callers disable submission while a custom duration is empty or invalid,
	// so an earlier preset can't be authorized by accident.
	$effect(() => {
		valid = !custom || parseDuration(customText) !== null;
	});

	const customInvalid = $derived(
		custom && customText.trim() !== '' && parseDuration(customText) === null
	);
	const options = $derived(
		presets.map((c) => ({
			value: c.seconds,
			label: c.label,
			selected: !custom && value === c.seconds
		}))
	);
</script>

<div class="space-y-3" data-slot="duration-picker">
	<ChipGroup
		{options}
		{value}
		{label}
		onchange={(s) => {
			custom = false;
			value = s;
		}}
	>
		{#snippet after({ chipClass })}
			<button
				type="button"
				role="radio"
				aria-checked={custom}
				tabindex={custom ? 0 : -1}
				data-index={options.length}
				class={chipClass(custom)}
				onclick={() => {
					custom = true;
					customText = '';
				}}>Custom</button
			>
		{/snippet}
	</ChipGroup>
	{#if custom}
		<div class="flex flex-wrap items-center gap-3">
			<Input
				{id}
				value={customText}
				oninput={onCustom}
				placeholder="e.g. 90m, 2h, 3d"
				aria-label="Custom duration"
				aria-invalid={customInvalid}
				aria-describedby="{id}-hint"
				class="max-w-44"
				autocomplete="off"
			/>
			<p
				id="{id}-hint"
				class="text-footnote {customInvalid ? 'text-destructive' : 'text-muted-foreground'}"
			>
				{customInvalid
					? 'Use minutes, hours, days or weeks, like 90m or 2d.'
					: formatDuration(value)}
			</p>
		</div>
	{/if}
	{#if isLong(value)}
		<p class="text-footnote text-warning" data-testid="long-note">
			That's longer than a day. Make sure that's what you want.
		</p>
	{/if}
</div>
