<script lang="ts">
	// A command or message to copy, in a monospace well with a copy button.
	import Copy from '@lucide/svelte/icons/copy';
	import Check from '@lucide/svelte/icons/check';
	import { Button } from '#lib/components/ui/button/index.js';

	let {
		value,
		label = 'Copy',
		caption
	}: {
		value: string;
		/** Accessible name of the copy button. */
		label?: string;
		/** Visible label above the well. */
		caption?: string;
	} = $props();
	let copied = $state(false);
	const id = `copy-${Math.random().toString(36).slice(2)}`;

	async function copy() {
		try {
			await navigator.clipboard.writeText(value);
		} catch {
			// Clipboard API unavailable (non-secure context): select the text.
			const el = document.getElementById(id);
			if (el) window.getSelection()?.selectAllChildren(el);
			return;
		}
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<div data-slot="copy-field">
	{#if caption}<p class="mb-1.5 px-1 text-footnote text-muted-foreground">{caption}</p>{/if}
	<div
		class="flex items-start gap-2 rounded-lg border border-border bg-background/60 py-2 pr-1.5 pl-3"
	>
		<code {id} class="min-w-0 flex-1 py-1 text-[13px] wrap-anywhere whitespace-pre-wrap select-all"
			>{value}</code
		>
		<Button variant="ghost" size="icon-sm" onclick={copy} aria-label={label}>
			{#if copied}<Check class="text-success" />{:else}<Copy />{/if}
		</Button>
	</div>
	<span class="sr-only" aria-live="polite">{copied ? 'Copied' : ''}</span>
</div>
