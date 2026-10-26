<script lang="ts">
	// Agent-written text: always plain text (never {@html}), shown in full,
	// wrapped, and labelled as unverified.
	let {
		text,
		label = 'Reason (written by the agent, unverified)'
	}: { text: string; label?: string } = $props();
</script>

<figure class="space-y-1.5">
	<figcaption class="text-sm text-muted-foreground">{label}</figcaption>
	<blockquote class="unverified-text" data-testid="unverified-text">{text}</blockquote>
</figure>
