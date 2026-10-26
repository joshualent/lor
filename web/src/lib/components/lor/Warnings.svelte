<script lang="ts">
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import type { Warning } from '#lib/api/client.js';

	// Warnings come from the server; this component only shows those that
	// apply to the level being granted.
	let { warnings, level }: { warnings: Warning[]; level: string } = $props();
	const shown = $derived(
		warnings.filter((w) => w.levels.includes(level as Warning['levels'][number]))
	);
</script>

{#if shown.length}
	<ul class="space-y-3" aria-label="Warnings">
		{#each shown as w (w.code)}
			<li
				class="rounded-lg border p-3 {w.severity === 'danger'
					? 'border-destructive/40 bg-destructive/8'
					: 'border-warning/40 bg-warning/8'}"
				data-warning={w.code}
			>
				<p
					class="flex items-center gap-2 font-medium {w.severity === 'danger'
						? 'text-destructive'
						: 'text-warning'}"
				>
					<TriangleAlert class="size-4 shrink-0" aria-hidden="true" />
					{w.title}
				</p>
				<p class="mt-1 text-sm text-foreground/90">{w.message}</p>
			</li>
		{/each}
	</ul>
{/if}
