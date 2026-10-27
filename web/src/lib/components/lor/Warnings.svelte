<script lang="ts">
	// Server-computed warnings, shown as is (title and message from the
	// server; severity picks the tone). Only those that apply to the level
	// being granted are shown. The UI never decides what's dangerous.
	import type { Component } from 'svelte';
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import Layers from '@lucide/svelte/icons/layers';
	import Crown from '@lucide/svelte/icons/crown';
	import type { Warning } from '#lib/api/client.js';
	import type { Tone } from '#lib/design/variants.js';
	import Callout from './Callout.svelte';

	let { warnings, level }: { warnings: Warning[]; level: string } = $props();
	const shown = $derived(
		warnings.filter((w) => w.levels.includes(level as Warning['levels'][number]))
	);

	const ICONS: Record<string, Component> = {
		lor_infra: ShieldAlert,
		hypervisor: Layers,
		root_access: Crown
	};
	const TONES: Record<Warning['severity'], Tone> = {
		danger: 'danger',
		warning: 'warning',
		info: 'info'
	};
</script>

{#if shown.length}
	<ul class="space-y-2.5" aria-label="Warnings" data-slot="warnings">
		{#each shown as w (w.code)}
			<li data-warning={w.code}>
				<Callout tone={TONES[w.severity] ?? 'warning'} icon={ICONS[w.code]} title={w.title}>
					{w.message}
				</Callout>
			</li>
		{/each}
	</ul>
{/if}
