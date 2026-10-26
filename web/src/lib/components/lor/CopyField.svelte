<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import Check from '@lucide/svelte/icons/check';
	import { Button } from '#lib/components/ui/button/index.js';

	let { value, label = 'Copy' }: { value: string; label?: string } = $props();
	let copied = $state(false);

	async function copy() {
		try {
			await navigator.clipboard.writeText(value);
		} catch {
			// Clipboard API unavailable (non-secure context): select the text.
			const el = document.getElementById(id) as HTMLElement | null;
			if (el) window.getSelection()?.selectAllChildren(el);
			return;
		}
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
	const id = `copy-${Math.random().toString(36).slice(2)}`;
</script>

<div class="flex items-start gap-2 rounded-lg bg-muted p-3">
	<code {id} class="min-w-0 flex-1 text-[13px] break-all whitespace-pre-wrap select-all"
		>{value}</code
	>
	<Button variant="ghost" size="icon-sm" onclick={copy} aria-label={label}>
		{#if copied}<Check class="text-success" />{:else}<Copy />{/if}
	</Button>
</div>
