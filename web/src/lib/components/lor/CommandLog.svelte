<script lang="ts">
	// Commands the agent's CLI reported (newest first), as plain text.
	import SquareTerminal from '@lucide/svelte/icons/square-terminal';
	import type { Schemas } from '#lib/api/client.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatArgv, formatClock, formatTimestamp } from '#lib/format.js';
	import EmptyState from './EmptyState.svelte';

	let {
		commands,
		ended = false
	}: {
		commands: Schemas['CommandEntry'][];
		/** The grant is over: say nothing was reported, don't promise more. */
		ended?: boolean;
	} = $props();
</script>

{#if commands.length === 0 && ended}
	<p class="rounded-xl bg-card px-4 py-3.5 text-callout text-muted-foreground">
		No commands were reported.
	</p>
{:else if commands.length === 0}
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
			<li class="flex flex-col gap-0.5 px-4 py-2.5 sm:flex-row sm:items-baseline sm:gap-3">
				<time
					class="shrink-0 text-footnote text-muted-foreground nums sm:w-[4.5rem]"
					datetime={c.reported_at}
					title={formatTimestamp(Date.parse(c.reported_at))}
					>{formatClock(Date.parse(c.reported_at), clock.now).replace(/\s\S+$/, '')}</time
				>
				<code
					class="min-w-0 flex-1 font-mono text-callout wrap-anywhere whitespace-pre-wrap {c.argv
						.length === 0
						? 'text-muted-foreground italic'
						: 'text-foreground'}">{formatArgv(c.argv)}</code
				>
			</li>
		{/each}
	</ol>
{/if}
