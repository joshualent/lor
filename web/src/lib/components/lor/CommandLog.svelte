<script lang="ts">
	// Commands the agent's CLI reported (newest first), as plain text.
	import SquareTerminal from '@lucide/svelte/icons/square-terminal';
	import type { Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatArgv, formatClock, formatTimestamp } from '#lib/format.js';
	import EmptyState from './EmptyState.svelte';

	let { commands }: { commands: Schemas['CommandEntry'][] } = $props();
</script>

{#if commands.length === 0}
	<EmptyState
		icon={SquareTerminal}
		title="No commands yet"
		description="Commands appear here as lor ssh reports them."
	/>
{:else}
	<ol
		class="divide-y divide-separator overflow-hidden rounded-xl bg-card"
		data-testid="command-log"
	>
		{#each commands as c (c.id)}
			<li class="flex items-baseline gap-3 px-4 py-2.5">
				<time
					class="w-[4.5rem] shrink-0 text-footnote text-muted-foreground nums"
					datetime={c.reported_at}
					title={formatTimestamp(Date.parse(c.reported_at))}
					>{formatClock(Date.parse(c.reported_at), clock.now).replace(/\s\S+$/, '')}</time
				>
				<code
					class="min-w-0 flex-1 font-mono text-callout break-all whitespace-pre-wrap {c.argv
						.length === 0
						? 'text-muted-foreground italic'
						: 'text-foreground'}">{formatArgv(c.argv)}</code
				>
			</li>
		{/each}
	</ol>
{/if}
