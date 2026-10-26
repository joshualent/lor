<script lang="ts">
	import Plus from '@lucide/svelte/icons/plus';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, call, type Level } from '#lib/api/client.js';
	import { errorMessage } from '#lib/errors.js';
	import { keys, fetchHosts } from '#lib/queries.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';

	const qc = useQueryClient();
	const hosts = createQuery(() => ({ queryKey: keys.hosts, queryFn: fetchHosts }));

	let open = $state(false);
	let name = $state('');
	let address = $state('');
	let maxLevel = $state<Level>('sudo');
	let hypervisor = $state(false);
	let lorInfra = $state(false);
	let busy = $state(false);

	async function add() {
		busy = true;
		try {
			const res = await call(
				api.POST('/hosts', {
					body: {
						name: name.trim(),
						address: address.trim(),
						max_level: maxLevel,
						hypervisor,
						lor_infra: lorInfra
					}
				})
			);
			if (res.scan_error)
				toast.warning(`Added ${res.host.name}, but the host key scan failed: ${res.scan_error}`);
			else toast.success(`Added ${res.host.name}`);
			qc.invalidateQueries({ queryKey: keys.hosts });
			open = false;
			await goto(`/hosts/${res.host.name}`);
		} catch (e) {
			toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}

	function flags(h: { hypervisor: boolean; lor_infra: boolean; disabled: boolean }) {
		return [
			h.hypervisor && 'hypervisor',
			h.lor_infra && 'Lor infrastructure',
			h.disabled && 'disabled'
		]
			.filter(Boolean)
			.join(' · ');
	}
</script>

<PageHeader title="Hosts">
	{#snippet actions()}<Button onclick={() => (open = true)}><Plus /> Add host</Button>{/snippet}
</PageHeader>

<Section>
	{#if hosts.isPending}
		<p class="text-muted-foreground">Loading…</p>
	{:else if hosts.data?.length === 0}
		<p class="text-muted-foreground">No hosts yet. Add one, then run its bootstrap one-liner.</p>
	{:else if hosts.data}
		<ul class="-my-2 divide-y divide-border">
			{#each hosts.data as h (h.id)}
				<li>
					<a href="/hosts/{h.name}" class="flex items-center gap-3 py-3 no-underline!">
						<div class="min-w-0 flex-1">
							<p>
								<span class="font-medium">{h.name}</span>
								<span class="font-mono text-sm text-muted-foreground">{h.address}</span>
							</p>
							<p class="text-sm text-muted-foreground">
								up to {h.max_level === 'sudo' ? 'sudo' : 'user'}{flags(h)
									? ` · ${flags(h)}`
									: ''}{h.host_key ? '' : ' · no host key yet'}
							</p>
						</div>
						<ChevronRight class="size-4 text-muted-foreground" />
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</Section>

<Confirm bind:open title="Add host" description="Lor scans its ed25519 host key right away.">
	<form
		class="space-y-4"
		id="add-host"
		onsubmit={(e) => {
			e.preventDefault();
			add();
		}}
	>
		<div class="space-y-2">
			<Label for="h-name">Name</Label>
			<Input
				id="h-name"
				bind:value={name}
				placeholder="nas"
				autocapitalize="off"
				autocomplete="off"
				required
			/>
			<p class="text-sm text-muted-foreground">
				Lowercase letters, digits and dashes. It becomes the certificate principal.
			</p>
		</div>
		<div class="space-y-2">
			<Label for="h-addr">LAN IP</Label>
			<Input
				id="h-addr"
				bind:value={address}
				placeholder="192.168.1.20"
				inputmode="decimal"
				autocomplete="off"
				required
			/>
			<p class="text-sm text-muted-foreground">
				An IP, not a name, so traffic always stays on the LAN.
			</p>
		</div>
		<fieldset class="space-y-2">
			<legend class="mb-1 text-sm font-medium">Most access allowed</legend>
			<label class="flex items-center gap-2"
				><input type="radio" bind:group={maxLevel} value="sudo" class="size-4 accent-primary" /> sudo</label
			>
			<label class="flex items-center gap-2"
				><input type="radio" bind:group={maxLevel} value="user" class="size-4 accent-primary" /> standard
				user only</label
			>
		</fieldset>
		<label class="flex items-start gap-3"
			><Checkbox bind:checked={hypervisor} class="mt-0.5" /><span>Hypervisor (Proxmox host)</span
			></label
		>
		<label class="flex items-start gap-3"
			><Checkbox bind:checked={lorInfra} class="mt-0.5" /><span
				>Lor infrastructure (runs Lor, or the Caddy proxy)</span
			></label
		>
	</form>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
		<Button type="submit" form="add-host" disabled={busy || !name || !address}
			>{busy ? 'Adding…' : 'Add host'}</Button
		>
	{/snippet}
</Confirm>
