<script lang="ts">
	// Component gallery (dev builds only): every component and variant of the
	// design system, with sample data. See docs/design-system.md.
	import Plus from '@lucide/svelte/icons/plus';
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Server from '@lucide/svelte/icons/server';
	import Inbox from '@lucide/svelte/icons/inbox';
	import Layers from '@lucide/svelte/icons/layers';
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import Bot from '@lucide/svelte/icons/bot';
	import FileBadge from '@lucide/svelte/icons/file-badge';
	import Fingerprint from '@lucide/svelte/icons/fingerprint';
	import Bell from '@lucide/svelte/icons/bell';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import Rows3 from '@lucide/svelte/icons/rows-3';
	import ListIcon from '@lucide/svelte/icons/list';
	import { ApiError } from '#lib/api/error.js';
	import type { Schemas, Warning } from '#lib/api/client.js';
	import type { AuditEvent } from '#lib/queries.js';
	import type { Tone } from '#lib/design/variants.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Checkbox } from '#lib/components/ui/checkbox/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { NativeSelect } from '#lib/components/ui/native-select/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import ActionBar from '#lib/components/lor/ActionBar.svelte';
	import AuditEventRow from '#lib/components/lor/AuditEventRow.svelte';
	import Callout from '#lib/components/lor/Callout.svelte';
	import Card from '#lib/components/lor/Card.svelte';
	import ChipGroup from '#lib/components/lor/ChipGroup.svelte';
	import ChoiceCard from '#lib/components/lor/ChoiceCard.svelte';
	import CommandLog from '#lib/components/lor/CommandLog.svelte';
	import Confirm from '#lib/components/lor/Confirm.svelte';
	import CopyField from '#lib/components/lor/CopyField.svelte';
	import Countdown from '#lib/components/lor/Countdown.svelte';
	import DescriptionItem from '#lib/components/lor/DescriptionItem.svelte';
	import DescriptionList from '#lib/components/lor/DescriptionList.svelte';
	import DurationPicker from '#lib/components/lor/DurationPicker.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import IconTile from '#lib/components/lor/IconTile.svelte';
	import LevelBadge from '#lib/components/lor/LevelBadge.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListRow from '#lib/components/lor/ListRow.svelte';
	import PageSkeleton from '#lib/components/lor/PageSkeleton.svelte';
	import ListSkeleton from '#lib/components/lor/ListSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import SearchField from '#lib/components/lor/SearchField.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import SegmentedControl from '#lib/components/lor/SegmentedControl.svelte';
	import SettingsRow from '#lib/components/lor/SettingsRow.svelte';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';
	import StatusBadge from '#lib/components/lor/StatusBadge.svelte';
	import SummaryCard from '#lib/components/lor/SummaryCard.svelte';
	import StatusDot from '#lib/components/lor/StatusDot.svelte';
	import TimeBar from '#lib/components/lor/TimeBar.svelte';
	import Timeline from '#lib/components/lor/Timeline.svelte';
	import UnverifiedText from '#lib/components/lor/UnverifiedText.svelte';
	import Warnings from '#lib/components/lor/Warnings.svelte';

	const TONES: Tone[] = ['neutral', 'info', 'success', 'warning', 'danger', 'violet', 'teal'];
	const SURFACES = [
		'background',
		'card',
		'card-hover',
		'popover',
		'secondary',
		'border',
		'separator'
	];
	const SIZES = [
		['title', 'Page title · 30/650'],
		['headline', 'Headline · 18/600'],
		['heading', 'Section heading · 15/600'],
		['body', 'Body · 15/400'],
		['callout', 'Callout · 14/400'],
		['footnote', 'Footnote · 13/400'],
		['caption', 'Caption · 12/500']
	] as const;
	const SIZE_CLASS: Record<string, string> = {
		title: 'text-title',
		headline: 'text-headline',
		heading: 'text-heading',
		body: 'text-body',
		callout: 'text-callout',
		footnote: 'text-footnote',
		caption: 'text-caption'
	};

	const now = Date.now();
	const iso = (deltaMs: number) => new Date(now + deltaMs).toISOString();
	const MIN = 60_000;

	const warnings: Warning[] = [
		{
			code: 'lor_infra',
			severity: 'danger',
			levels: ['user', 'sudo'],
			title: 'caddy is Lor infrastructure',
			message:
				"caddy runs Lor or proxies its web UI. Root here can read Lor's signing key and give itself access to every host."
		},
		{
			code: 'root_access',
			severity: 'danger',
			levels: ['sudo'],
			title: 'Root access (sudo)',
			message: 'Logs in as agent-sudo. Can read, change or delete anything on caddy.'
		},
		{
			code: 'hypervisor',
			severity: 'danger',
			levels: ['sudo'],
			title: 'pve is a hypervisor',
			message: 'Sudo on pve controls every container and VM on it.'
		}
	];

	const events: AuditEvent[] = [
		{
			id: 9,
			ts: iso(-2 * MIN),
			actor: 'agent:claude',
			action: 'grant.requested',
			grant_id: 12,
			details: { level: 'sudo', duration_s: 7200 }
		},
		{
			id: 8,
			ts: iso(-6 * MIN),
			actor: 'admin:iPhone',
			action: 'grant.authorized',
			grant_id: 11,
			details: { level: 'user', duration_s: 3600 }
		},
		{
			id: 7,
			ts: iso(-7 * MIN),
			actor: 'system',
			action: 'cert.issued',
			grant_id: 11,
			details: { certificate_id: 4 }
		},
		{
			id: 6,
			ts: iso(-30 * MIN),
			actor: 'admin:iPhone',
			action: 'grant.denied',
			grant_id: 10,
			details: { note: 'Use the Proxmox UI.' }
		},
		{
			id: 5,
			ts: iso(-50 * MIN),
			actor: 'system',
			action: 'grant.expired',
			grant_id: 9,
			details: {}
		},
		{
			id: 4,
			ts: iso(-80 * MIN),
			actor: 'admin:iPhone',
			action: 'host.key_changed_detected',
			details: { host: 'nas' }
		},
		{
			id: 3,
			ts: iso(-90 * MIN),
			actor: 'admin:iPhone',
			action: 'agent.enrolled',
			details: { name: 'claude' }
		},
		{ id: 2, ts: iso(-95 * MIN), actor: 'admin:MacBook', action: 'admin.login', details: {} }
	];

	const commands: Schemas['CommandEntry'][] = [
		{
			id: 3,
			argv: ['sh', '-c', 'dmesg | grep -i "ata3" | tail -n 50'],
			reported_at: iso(-1 * MIN)
		},
		{ id: 2, argv: ['smartctl', '-a', '/dev/sdb'], reported_at: iso(-3 * MIN) },
		{ id: 1, argv: [], reported_at: iso(-5 * MIN) }
	];

	const hostile = `<script>alert(1)</${'script'}><b>bold</b> Check the ZFS pool & SMART data.\nSecond line.`;

	let segment = $state('all');
	let segment2 = $state('list');
	let chip = $state<string | null>('grant.');
	let duration = $state(3600);
	let level = $state<'user' | 'sudo'>('sudo');
	let switchOn = $state(true);
	let switchOff = $state(false);
	let checked = $state(false);
	let search = $state('nas');
	let confirmOpen = $state(false);
	let select = $state('claude');
</script>

<PageHeader
	title="Component gallery"
	subtitle="Every component and variant in Lor's design system. Dev builds only."
	documentTitle="Gallery"
>
	{#snippet actions()}<Button variant="secondary" href="/">Back to the app</Button>{/snippet}
</PageHeader>

<div class="space-y-12">
	<!-- ------------------------------------------------------------- tokens -->
	<Section title="Linked summaries">
		<div class="grid grid-cols-3 gap-2 sm:gap-4">
			<SummaryCard
				href="#"
				label="Waiting for you"
				value={4}
				description="Requests to review"
				icon={Inbox}
				tone="warning"
			/>
			<SummaryCard
				href="#"
				label="Active access"
				value={3}
				description="Grants in progress"
				icon={Server}
				tone="success"
			/>
			<SummaryCard
				href="#"
				label="Open offers"
				value={0}
				description="Ready to claim"
				icon={KeyRound}
				tone="info"
			/>
		</div>
	</Section>
	<Section title="Surfaces" description="Elevation by lightness, not shadow.">
		<div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
			{#each SURFACES as s (s)}
				<div class="overflow-hidden rounded-xl border border-border">
					<div class="h-12" style="background: var(--{s})"></div>
					<p class="bg-card px-3 py-2 font-mono text-caption">--{s}</p>
				</div>
			{/each}
		</div>
	</Section>

	<Section title="Semantic tones" description="Color always carries meaning.">
		<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
			{#each TONES as t (t)}
				<div class="flex items-center gap-3 rounded-xl bg-card px-3 py-2.5">
					<IconTile icon={Bell} tone={t} size="sm" />
					<Badge tone={t}>{t}</Badge>
					<StatusDot tone={t} />
					<span class="ml-auto font-mono text-caption text-muted-foreground"
						>--{t === 'danger' ? 'destructive' : t}[-tint|-edge]</span
					>
				</div>
			{/each}
		</div>
	</Section>

	<Section title="Type scale">
		<Card class="space-y-3">
			{#each SIZES as [name, label] (name)}
				<div class="flex flex-wrap items-baseline justify-between gap-2">
					<span class={SIZE_CLASS[name]}>{label}</span>
					<code class="text-caption text-muted-foreground">text-{name}</code>
				</div>
			{/each}
			<p class="font-mono text-callout">Monospace: lor ssh nas -- zpool status -v</p>
		</Card>
	</Section>

	<!-- ------------------------------------------------------------ buttons -->
	<Section title="Buttons" description="One primary (accent) action per view.">
		<Card class="space-y-4">
			<div class="flex flex-wrap gap-2">
				<Button><Plus /> Primary</Button>
				<Button variant="secondary">Secondary</Button>
				<Button variant="outline">Outline</Button>
				<Button variant="ghost">Ghost</Button>
				<Button variant="destructive">Destructive</Button>
				<Button variant="link">Link</Button>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<Button size="lg"><KeyRound /> Large (passkey)</Button>
				<Button size="sm">Small</Button>
				<Button size="icon" variant="ghost" aria-label="Edit"><Pencil /></Button>
				<Button size="icon-sm" variant="ghost" aria-label="Delete" class="text-destructive"
					><Trash2 /></Button
				>
				<Button disabled>Disabled</Button>
				<Button disabled><Spinner /> Waiting for passkey…</Button>
			</div>
		</Card>
	</Section>

	<!-- ------------------------------------------------------------- status -->
	<Section title="Status" description="Badges only for states worth noticing.">
		<Card class="space-y-4">
			<div class="flex flex-wrap gap-2">
				{#each ['pending', 'offered', 'active', 'denied', 'revoked', 'expired', 'released', 'request_timeout', 'offer_expired'] as s (s)}
					<StatusBadge status={s} />
				{/each}
			</div>
			<div class="flex flex-wrap gap-2">
				{#each ['pending', 'approved', 'denied', 'request_timeout', 'moot'] as s (s)}
					<StatusBadge status={s} kind="extension" />
				{/each}
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<LevelBadge level="sudo" />
				<LevelBadge level="user" />
				<LevelBadge level="sudo" short />
				<Badge tone="danger" size="md">Lor infra</Badge>
				<Badge tone="warning" size="md">Hypervisor</Badge>
				<StatusDot tone="warning" pulse label="Waiting" />
			</div>
			<div class="flex flex-wrap items-center gap-3">
				<Countdown end={iso(90 * MIN)} variant="badge" />
				<Countdown end={iso(4 * MIN)} variant="badge" />
				<Countdown end={iso(40_000)} variant="badge" />
				<Countdown end={iso(-1000)} variant="badge" />
				<Countdown end={iso(4 * MIN)} />
			</div>
			<div class="space-y-2">
				<TimeBar start={iso(-30 * MIN)} end={iso(90 * MIN)} />
				<TimeBar start={iso(-56 * MIN)} end={iso(4 * MIN)} />
			</div>
		</Card>
	</Section>

	<Section title="Icon tiles">
		<Card class="flex flex-wrap items-center gap-3">
			<IconTile icon={Server} />
			<IconTile icon={Layers} tone="warning" />
			<IconTile icon={ShieldAlert} tone="danger" />
			<IconTile icon={Inbox} tone="warning" />
			<IconTile icon={FileBadge} tone="teal" />
			<IconTile icon={Bot} tone="violet" />
			<IconTile text="claude" tone="violet" />
			<IconTile icon={KeyRound} tone="info" size="sm" />
			<IconTile icon={Server} size="lg" />
		</Card>
	</Section>

	<!-- -------------------------------------------------------------- lists -->
	<Section title="Grouped list" count={5} description="Link, button, static and with controls.">
		{#snippet action()}<Button size="sm" variant="ghost">Action</Button>{/snippet}
		<List>
			<ListRow href="#lists" title="nas" subtitle="192.168.1.20 · up to sudo">
				{#snippet leading()}<IconTile icon={Server} />{/snippet}
			</ListRow>
			<ListRow href="#lists" title="pve" subtitle="192.168.1.10 · up to sudo">
				{#snippet leading()}<IconTile icon={Layers} tone="warning" />{/snippet}
				{#snippet metaContent()}<Badge tone="warning">Hypervisor</Badge>{/snippet}
			</ListRow>
			<ListRow title="Static row" subtitle="No chevron, not interactive" meta="12:04">
				{#snippet leading()}<IconTile icon={Fingerprint} tone="teal" />{/snippet}
			</ListRow>
			<ListRow title="iPhone" subtitle="With trailing controls">
				{#snippet leading()}<IconTile icon={Smartphone} tone="success" />{/snippet}
				{#snippet trailing()}
					<Button size="icon-sm" variant="ghost" aria-label="Rename"><Pencil /></Button>
					<Button size="icon-sm" variant="ghost" aria-label="Remove" class="text-destructive"
						><Trash2 /></Button
					>
				{/snippet}
			</ListRow>
			<ListRow
				href="#"
				compactOnNarrow
				title="codex on media"
				subtitle="A compact request preview on phones."
				density="relaxed"
				>{#snippet leading()}<IconTile icon={Inbox} tone="warning" />{/snippet}</ListRow
			>
			<ListRow onclick={() => {}} title="Sign out everywhere" tone="danger" density="compact">
				{#snippet leading()}<IconTile icon={ShieldAlert} tone="danger" size="sm" />{/snippet}
			</ListRow>
		</List>
	</Section>

	<Section title="Description list">
		<DescriptionList>
			<DescriptionItem label="Host"
				>nas <span class="font-mono text-callout text-muted-foreground">192.168.1.20</span
				></DescriptionItem
			>
			<DescriptionItem label="Public URL" layout="responsive" mono
				>https://lor.example.com</DescriptionItem
			>
			<DescriptionItem label="Connect from" mono>192.168.1.50</DescriptionItem>
			<DescriptionItem label="Key" layout="stacked" mono
				>SHA256:9Qk2m4b8m2J7yN1fS0rP3cV6xZ5wT8uH4gL2aE1dKc</DescriptionItem
			>
		</DescriptionList>
	</Section>

	<Section title="Settings rows">
		<ul class="divide-y divide-separator overflow-hidden rounded-xl bg-card">
			<SettingsRow
				title="Hypervisor"
				description="A Proxmox host."
				icon={Layers}
				tone="warning"
				for="g-sw1"
			>
				{#snippet control()}<Switch id="g-sw1" bind:checked={switchOn} />{/snippet}
			</SettingsRow>
			<SettingsRow title="Lor infrastructure" icon={ShieldAlert} tone="danger" for="g-sw2">
				{#snippet control()}<Switch id="g-sw2" bind:checked={switchOff} />{/snippet}
			</SettingsRow>
			<SettingsRow title="Passkeys" description="2 registered" icon={KeyRound} tone="info">
				{#snippet control()}<Button size="sm" variant="secondary">Manage</Button>{/snippet}
			</SettingsRow>
		</ul>
	</Section>

	<Section title="Audit events and timeline">
		<div class="space-y-4">
			<List ordered>
				{#each events as ev (ev.id)}<AuditEventRow event={ev} />{/each}
			</List>
			<Timeline events={events.slice(0, 4).reverse()} />
		</div>
	</Section>

	<Section title="Command log" footnote="Reported by the agent's CLI (sample).">
		<div class="space-y-4">
			<CommandLog {commands} />
			<CommandLog commands={[]} />
		</div>
	</Section>

	<!-- ---------------------------------------------------------- controls -->
	<Section title="Filters">
		<div class="space-y-3">
			<SegmentedControl
				label="Who"
				bind:value={segment}
				options={[
					{ value: 'all', label: 'Everyone' },
					{ value: 'admin', label: 'You' },
					{ value: 'agent', label: 'Agents' },
					{ value: 'system', label: 'System' }
				]}
			/>
			<SegmentedControl
				label="View"
				bind:value={segment2}
				options={[
					{ value: 'list', label: 'List', icon: ListIcon },
					{ value: 'grouped', label: 'Grouped', icon: Rows3 }
				]}
			/>
			<ChipGroup
				label="Category"
				scroll
				bind:value={chip}
				options={[
					{ value: '', label: 'All' },
					{ value: 'grant.', label: 'Grants' },
					{ value: 'host.', label: 'Hosts' },
					{ value: 'agent.', label: 'Agents' },
					{ value: 'admin.', label: 'Sign-ins' }
				]}
			/>
			<SearchField bind:value={search} label="Filter hosts" placeholder="Filter by name or IP" />
		</div>
	</Section>

	<Section title="Form controls">
		<Card class="space-y-5">
			<Field label="Name" for="g-name" hint="Lowercase letters, digits and dashes.">
				<Input id="g-name" placeholder="nas" />
			</Field>
			<Field label="Address" for="g-addr" error="Use an IP address, not a name.">
				<Input id="g-addr" value="nas.local" aria-invalid="true" />
			</Field>
			<Field label="Agent" for="g-agent">
				<NativeSelect id="g-agent" bind:value={select}>
					<option value="claude">claude</option>
					<option value="codex">codex</option>
				</NativeSelect>
			</Field>
			<Field label="Note" for="g-note" optional>
				<Textarea id="g-note" rows={2} placeholder="What it's for" />
			</Field>
			<label class="flex items-start gap-3">
				<Checkbox bind:checked id="g-check" class="mt-0.5" />
				<span class="text-callout">I understand this gives root on nas</span>
			</label>
		</Card>
	</Section>

	<Section title="Choice cards and durations">
		<div class="space-y-3">
			<div class="space-y-2" role="radiogroup" aria-label="Level">
				<ChoiceCard
					name="g-level"
					value="sudo"
					bind:group={level}
					title="sudo (root)"
					tone="warning">Logs in as <code>agent-sudo</code>.</ChoiceCard
				>
				<ChoiceCard name="g-level" value="user" bind:group={level} title="Standard user"
					>Logs in as <code>agent</code>, no sudo.</ChoiceCard
				>
			</div>
			<Card><DurationPicker bind:value={duration} id="g-duration" /></Card>
		</div>
	</Section>

	<!-- ---------------------------------------------------------- messages -->
	<Section title="Callouts">
		<div class="space-y-2.5">
			{#each TONES.slice(0, 5) as t (t)}
				<Callout tone={t} title="{t[0].toUpperCase()}{t.slice(1)} callout"
					>One or two sentences that explain what this means and what to do.</Callout
				>
			{/each}
		</div>
	</Section>

	<Section title="Server warnings" description="Shown as is; severity picks the tone.">
		<Warnings {warnings} level="sudo" />
	</Section>

	<Section title="Agent-written text">
		<UnverifiedText text={hostile} />
	</Section>

	<Section title="Copy field">
		<Card>
			<CopyField
				caption="Install"
				value="curl -fsSL 'https://lor.example.com/bootstrap/nas.sh?t=abc123' | ssh nas sudo sh"
			/>
		</Card>
	</Section>

	<!-- ------------------------------------------------------------- states -->
	<Section title="Empty, error and loading">
		<div class="space-y-4">
			<EmptyState icon={Server} title="No hosts yet" description="Add a host by its LAN IP.">
				{#snippet action()}<Button variant="secondary"><Plus /> Add host</Button>{/snippet}
			</EmptyState>
			<ErrorState
				variant="inset"
				error={new ApiError(0, 'network', "Can't reach Lor. Check your connection and try again.")}
				onretry={() => {}}
			/>
			<ErrorState
				variant="inset"
				error={new ApiError(404, 'not_found', 'grant not found')}
				back={{ href: '#', label: 'Home' }}
			/>
			<StaleNotice
				error={new ApiError(0, 'network', "Can't reach Lor. Check your connection and try again.")}
				updatedAt={now - 2 * MIN}
				onretry={() => {}}
			/>
			<EmptyState
				size="compact"
				icon={Inbox}
				title="You're all caught up"
				description="New requests appear here for review."
			/>
			<PageSkeleton header={false} layout="dashboard" />
			<ListSkeleton rows={2} />
			<div class="flex items-center gap-3">
				<Skeleton shape="tile" />
				<Skeleton class="w-40" />
				<Skeleton shape="circle" class="size-6" />
			</div>
		</div>
	</Section>

	<Section title="Confirmations and action bar">
		<Card class="space-y-4">
			<p class="text-callout text-muted-foreground">
				A dialog on wide screens, a bottom drawer on phones.
			</p>
			<Button variant="secondary" onclick={() => (confirmOpen = true)}>Open confirmation</Button>
			<ActionBar>
				<Button size="lg" class="sm:flex-1"
					><KeyRound /> Authorize standard user on nas for 1 h</Button
				>
				<Button size="lg" variant="outline">Deny</Button>
			</ActionBar>
		</Card>
	</Section>
</div>

<Confirm
	bind:open={confirmOpen}
	title="Revoke access to nas?"
	description="lor ssh stops working right away."
>
	{#snippet footer()}
		<Button variant="outline" onclick={() => (confirmOpen = false)}>Cancel</Button>
		<Button variant="destructive" onclick={() => (confirmOpen = false)}>Revoke</Button>
	{/snippet}
</Confirm>
