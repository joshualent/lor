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

	let bodyElement = $state<HTMLDivElement | null>(null);
	let moreBelow = $state(false);
	$effect(() => {
		const el = bodyElement;
		if (!el) {
			moreBelow = false;
			return;
		}
		const update = () => {
			moreBelow = el.scrollHeight - el.scrollTop > el.clientHeight + 4;
		};
		const observer = new ResizeObserver(update);
		observer.observe(el);
		if (el.firstElementChild) observer.observe(el.firstElementChild);
		el.addEventListener('scroll', update);
		update();
		return () => {
			observer.disconnect();
			el.removeEventListener('scroll', update);
		};
	});
	const desktop = new MediaQuery('min-width: 768px');
</script>

{#if desktop.current}
	<Dialog.Root bind:open>
		<Dialog.Content class="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden">
			<Dialog.Header class="shrink-0 pr-6">
				<Dialog.Title>{title}</Dialog.Title>
				{#if description}<Dialog.Description>{description}</Dialog.Description>{/if}
			</Dialog.Header>
			{#if children}<div
					bind:this={bodyElement}
					class="-mx-1 min-h-0 flex-1 overflow-y-auto overscroll-contain px-1 py-1"
				>
					{@render children()}
				</div>{/if}
			{#if moreBelow}<p class="shrink-0 text-center text-caption text-muted-foreground">
					↓ Scroll for more details
				</p>{/if}
			<Dialog.Footer class="shrink-0 gap-2 border-t border-separator pt-4"
				>{@render footer()}</Dialog.Footer
			>
		</Dialog.Content>
	</Dialog.Root>
{:else}
	<Drawer.Root bind:open>
		<Drawer.Content class="max-h-[90dvh]">
			<!-- Header and footer stay put; only the body scrolls. -->
			<div class="mx-auto flex min-h-0 w-full max-w-lg flex-1 flex-col px-4">
				<Drawer.Header
					class="shrink-0 px-0 pt-5 text-left group-data-[vaul-drawer-direction=bottom]/drawer-content:text-left group-data-[vaul-drawer-direction=top]/drawer-content:text-left"
				>
					<Drawer.Title>{title}</Drawer.Title>
					{#if description}<Drawer.Description>{description}</Drawer.Description>{/if}
				</Drawer.Header>
				{#if children}
					<div
						bind:this={bodyElement}
						class="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-1"
					>
						{@render children()}
					</div>
				{/if}
				{#if moreBelow}<p class="shrink-0 pt-2 text-center text-caption text-muted-foreground">
						↓ Scroll for more details
					</p>{/if}
				<Drawer.Footer
					class="shrink-0 border-t border-separator px-0 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]"
					>{@render footer()}</Drawer.Footer
				>
			</div>
		</Drawer.Content>
	</Drawer.Root>
{/if}
