<script lang="ts">
	// Agent-written text: always plain text (never {@html}), shown in full,
	// wrapped, and labelled as unverified.
	import MessageSquareQuote from '@lucide/svelte/icons/message-square-quote';

	let {
		text,
		label = 'Reason (written by the agent, unverified)'
	}: { text: string; label?: string } = $props();
</script>

<figure data-slot="unverified-text">
	<figcaption class="mb-2 flex items-center gap-1.5 px-1 text-footnote text-muted-foreground">
		<MessageSquareQuote class="size-3.5 shrink-0" aria-hidden="true" />{label}
	</figcaption>
	<blockquote
		class="rounded-xl border-l-[3px] border-l-input bg-card px-4 py-3 text-body unverified-text"
		data-testid="unverified-text"
	>
		{text}
	</blockquote>
</figure>
