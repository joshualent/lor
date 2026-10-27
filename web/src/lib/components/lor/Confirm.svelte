<script lang="ts">
	// A confirmation that is a dialog on wide screens and a bottom drawer on
	// phones (reachable one-handed).
	import type { Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import * as Drawer from '#lib/components/ui/drawer/index.js';

	let {
		open = $bindable(false),
		title,
		description,
		children,
		footer
	}: {
		open: boolean;
		title: string;
		description?: string;
		children?: Snippet;
		footer: Snippet;
	} = $props();

	const desktop = new MediaQuery('min-width: 768px');
</script>

{#if desktop.current}
	<Dialog.Root bind:open>
		<Dialog.Content>
			<Dialog.Header>
				<Dialog.Title>{title}</Dialog.Title>
				{#if description}<Dialog.Description>{description}</Dialog.Description>{/if}
			</Dialog.Header>
			{@render children?.()}
			<Dialog.Footer class="gap-2">{@render footer()}</Dialog.Footer>
		</Dialog.Content>
	</Dialog.Root>
{:else}
	<Drawer.Root bind:open>
		<Drawer.Content>
			<div class="mx-auto w-full max-w-lg overflow-y-auto px-4 pb-2">
				<Drawer.Header class="px-0 pt-5 text-left">
					<Drawer.Title>{title}</Drawer.Title>
					{#if description}<Drawer.Description>{description}</Drawer.Description>{/if}
				</Drawer.Header>
				{@render children?.()}
				<Drawer.Footer class="px-0 pb-[max(1rem,env(safe-area-inset-bottom))]"
					>{@render footer()}</Drawer.Footer
				>
			</div>
		</Drawer.Content>
	</Drawer.Root>
{/if}
