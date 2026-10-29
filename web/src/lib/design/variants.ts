// Every styling variant in the app lives here (tailwind-variants), so pages
// compose components instead of hand-rolling Tailwind. See
// docs/design-system.md for when to use which tone.
import { createTV, type VariantProps } from 'tailwind-variants';
import { mergeConfig } from './merge';

const tv = createTV({ twMergeConfig: mergeConfig });

/**
 * Semantic tones. Color always carries meaning:
 * - success: active, approved
 * - warning: needs your decision, expiring soon, sudo (root)
 * - danger: Lor infrastructure, denied, revoked, destructive
 * - info: offers, informational notes
 * - neutral: ended normally, inactive, default
 * - violet / teal: audit categories only (never a status)
 */
export type Tone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'violet' | 'teal';

/** Text/icon color for a tone. */
export const toneText: Record<Tone, string> = {
	neutral: 'text-muted-foreground',
	info: 'text-info',
	success: 'text-success',
	warning: 'text-warning',
	danger: 'text-destructive',
	violet: 'text-violet',
	teal: 'text-teal'
};

/** Tinted fill + tone text, for badges and icon tiles. */
const toneTinted: Record<Tone, string> = {
	neutral: 'bg-neutral-tint text-neutral',
	info: 'bg-info-tint text-info',
	success: 'bg-success-tint text-success',
	warning: 'bg-warning-tint text-warning',
	danger: 'bg-destructive-tint text-destructive',
	violet: 'bg-violet-tint text-violet',
	teal: 'bg-teal-tint text-teal'
};

const toneDot: Record<Tone, string> = {
	neutral: 'bg-neutral/70',
	info: 'bg-info',
	success: 'bg-success',
	warning: 'bg-warning',
	danger: 'bg-destructive',
	violet: 'bg-violet',
	teal: 'bg-teal'
};

// ---------------------------------------------------------------- buttons

export const buttonVariants = tv({
	base: "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring aria-invalid:ring-destructive/40 aria-invalid:border-destructive/50 relative rounded-md border border-transparent bg-clip-padding text-sm font-medium aria-invalid:ring-3 active:not-aria-[haspopup]:translate-y-px [&_svg:not([class*='size-'])]:size-4 group/button inline-flex shrink-0 items-center justify-center whitespace-nowrap transition-[color,background-color,border-color,opacity] duration-(--duration-fast) outline-none select-none disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0",
	variants: {
		// One accent (default) used for the page's main action; quiet
		// alternatives; no shadows. Sizes are touch-friendly.
		variant: {
			default: 'bg-primary text-primary-foreground hover:bg-primary/90',
			outline: 'border-border bg-transparent hover:bg-secondary aria-expanded:bg-secondary',
			secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
			ghost: 'hover:bg-secondary aria-expanded:bg-secondary',
			destructive:
				'bg-destructive-tint text-destructive hover:bg-destructive-edge/70 focus-visible:ring-destructive/30',
			link: 'text-primary underline-offset-4 hover:underline focus-visible:underline'
		},
		size: {
			default: 'h-10 gap-2 px-4',
			sm: "h-8 gap-1.5 px-3 text-footnote after:absolute after:-inset-y-1.5 after:inset-x-0 after:content-['']",
			lg: 'h-12 gap-2 px-5 text-base',
			icon: 'size-10',
			'icon-sm': "size-8 after:absolute after:-inset-1.5 after:content-['']"
		}
	},
	compoundVariants: [{ variant: 'link', class: 'px-0' }],
	defaultVariants: { variant: 'default', size: 'default' }
});
export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
export type ButtonSize = VariantProps<typeof buttonVariants>['size'];

// ----------------------------------------------------------------- badges

export const badgeVariants = tv({
	base: 'inline-flex shrink-0 items-center gap-1 rounded-full font-medium whitespace-nowrap [&_svg]:pointer-events-none [&_svg]:shrink-0',
	variants: {
		tone: toneTinted,
		size: {
			sm: 'h-5 px-2 text-caption [&_svg]:size-3',
			md: 'h-6 px-2.5 text-footnote [&_svg]:size-3.5'
		}
	},
	defaultVariants: { tone: 'neutral', size: 'sm' }
});
export type BadgeVariants = VariantProps<typeof badgeVariants>;

// -------------------------------------------------------------- icon tile

export const iconTileVariants = tv({
	base: 'inline-flex shrink-0 items-center justify-center [&_svg]:pointer-events-none [&_svg]:shrink-0',
	variants: {
		tone: toneTinted,
		size: {
			sm: 'size-7 rounded-md [&_svg]:size-3.5 text-caption font-semibold',
			md: 'size-9 rounded-[0.6rem] [&_svg]:size-[1.125rem] text-footnote font-semibold',
			lg: 'size-12 rounded-xl [&_svg]:size-6 text-headline'
		}
	},
	defaultVariants: { tone: 'neutral', size: 'md' }
});
export type IconTileVariants = VariantProps<typeof iconTileVariants>;

// ------------------------------------------------------------- status dot

export const statusDotVariants = tv({
	base: 'relative inline-flex size-2 shrink-0 rounded-full',
	variants: {
		tone: toneDot,
		pulse: {
			true: 'before:absolute before:inset-0 before:animate-ping before:rounded-full before:bg-inherit before:opacity-60 motion-reduce:before:hidden',
			false: ''
		}
	},
	defaultVariants: { tone: 'neutral', pulse: false }
});

// ---------------------------------------------------------------- callout

export const calloutVariants = tv({
	slots: {
		root: 'flex gap-3 rounded-xl border p-3.5 text-left sm:p-4',
		icon: 'mt-0.5 size-[1.125rem] shrink-0',
		title: 'text-heading',
		body: 'mt-0.5 text-callout text-foreground/90'
	},
	variants: {
		tone: {
			neutral: { root: 'border-border bg-card', icon: 'text-muted-foreground' },
			info: { root: 'border-info-edge bg-info-tint', icon: 'text-info', title: 'text-info' },
			success: {
				root: 'border-success-edge bg-success-tint',
				icon: 'text-success',
				title: 'text-success'
			},
			warning: {
				root: 'border-warning-edge bg-warning-tint',
				icon: 'text-warning',
				title: 'text-warning'
			},
			danger: {
				root: 'border-destructive-edge bg-destructive-tint',
				icon: 'text-destructive',
				title: 'text-destructive'
			},
			violet: {
				root: 'border-violet-edge bg-violet-tint',
				icon: 'text-violet',
				title: 'text-violet'
			},
			teal: { root: 'border-teal-edge bg-teal-tint', icon: 'text-teal', title: 'text-teal' }
		}
	},
	defaultVariants: { tone: 'neutral' }
});

// ------------------------------------------------------------ lists, rows

/** The grouped ("inset") list surface. */
export const listVariants = tv({
	base: 'overflow-hidden rounded-xl border border-border bg-card',
	variants: {
		divided: { true: 'divide-y divide-separator', false: '' }
	},
	defaultVariants: { divided: true }
});

/**
 * A row in a grouped list. `main` is the link/button/div holding leading
 * content, text and trailing metadata; `trailing` holds separate controls
 * (buttons can't nest inside links), outside `main`.
 */
export const listRowVariants = tv({
	slots: {
		root: 'group/row relative flex items-stretch',
		main: 'flex min-w-0 flex-1 items-center gap-3 px-4 text-left',
		leading: 'flex shrink-0 items-center self-center',
		content: 'min-w-0 flex-1',
		title: 'block min-w-0 text-body font-medium break-words text-foreground',
		subtitle: 'mt-0.5 block min-w-0 text-callout break-words text-muted-foreground',
		meta: 'flex shrink-0 flex-col items-end gap-1 text-right text-footnote text-muted-foreground nums',
		chevron:
			'size-4 shrink-0 text-icon-muted transition-transform duration-(--duration-fast) ease-(--ease-standard)',
		trailing: 'flex shrink-0 items-center gap-3 pr-3'
	},
	variants: {
		interactive: {
			true: {
				main: 'focus-ring-inset cursor-pointer transition-colors duration-(--duration-fast) hover:bg-card-hover active:bg-card-hover no-underline!',
				chevron: 'group-hover/row:translate-x-0.5'
			},
			false: {}
		},
		compactOnNarrow: {
			true: {
				main: 'max-[359px]:grid max-[359px]:grid-cols-[minmax(0,1fr)_auto] max-[359px]:gap-x-2 max-[359px]:gap-y-0.5 max-[359px]:px-3 max-[359px]:py-2 max-[359px]:min-h-11',
				leading: 'max-[359px]:hidden',
				content: 'max-[359px]:col-start-1 max-[359px]:row-start-1',
				meta: 'max-[359px]:col-start-1 max-[359px]:row-start-2',
				chevron: 'max-[359px]:col-start-2 max-[359px]:row-start-1 max-[359px]:row-span-2'
			},
			false: {}
		},
		stackMeta: {
			true: {
				main: 'grid grid-cols-[auto_minmax(0,1fr)_auto] gap-y-1 sm:flex',
				leading: 'row-span-2',
				content: 'col-start-2 row-start-1',
				meta: 'col-start-2 row-start-2 flex-row flex-wrap items-center justify-start text-left sm:flex-col sm:items-end sm:text-right',
				chevron: 'col-start-3 row-span-2 row-start-1 self-center'
			},
			false: {}
		},
		hasTrailing: { true: { main: 'pr-2' }, false: {} },
		density: {
			default: { main: 'min-h-16 py-3.5' },
			compact: { main: 'min-h-11 py-2' },
			relaxed: { main: 'min-h-20 py-4' }
		},
		tone: {
			default: {},
			danger: { title: 'text-destructive' },
			accent: { title: 'text-primary' }
		},
		disabled: { true: { root: 'opacity-50', main: 'pointer-events-none' }, false: {} }
	},
	defaultVariants: {
		interactive: false,
		hasTrailing: false,
		density: 'default',
		tone: 'default',
		disabled: false
	}
});

// ------------------------------------------------ segmented control, chips

export const segmentedVariants = tv({
	slots: {
		root: 'inline-flex max-w-full items-stretch gap-0.5 rounded-lg bg-card p-0.5',
		item: "focus-ring relative inline-flex min-h-9 min-w-0 sm:min-h-8 after:absolute after:-inset-y-1 after:inset-x-0 after:content-[''] flex-1 items-center justify-center gap-1.5 rounded-md px-3 text-footnote font-medium whitespace-nowrap text-muted-foreground transition-colors duration-(--duration-fast) hover:text-foreground aria-checked:bg-secondary aria-checked:text-foreground before:absolute before:bottom-1 before:inset-x-3 before:h-0.5 before:rounded-full before:bg-primary before:opacity-0 aria-checked:before:opacity-100 [&_svg]:size-3.5"
	},
	variants: {
		fill: { true: { root: 'flex w-full' }, false: {} }
	},
	defaultVariants: { fill: false }
});

export const chipVariants = tv({
	base: "focus-ring relative inline-flex h-9 shrink-0 after:absolute after:-inset-y-1 after:inset-x-0 after:content-[''] items-center justify-center gap-1.5 rounded-full border px-3.5 text-callout whitespace-nowrap transition-colors duration-(--duration-fast) disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-3.5",
	variants: {
		selected: {
			true: 'border-primary/60 bg-info-tint text-primary',
			false: 'border-border text-foreground hover:bg-secondary'
		}
	},
	defaultVariants: { selected: false }
});

// ------------------------------------------------------- choice (radio) card

export const choiceCardVariants = tv({
	slots: {
		root: 'flex cursor-pointer gap-3 rounded-xl border p-3.5 transition-colors duration-(--duration-fast) has-focus-visible:outline-2 has-focus-visible:outline-offset-2 has-focus-visible:outline-ring',
		indicator:
			'mt-0.5 grid size-[1.125rem] shrink-0 place-items-center rounded-full border-2 transition-colors',
		dot: 'size-2 rounded-full bg-primary-foreground transition-transform'
	},
	variants: {
		selected: {
			true: { root: 'border-primary/70 bg-info-tint/60', indicator: 'border-primary bg-primary' },
			false: {
				root: 'border-border bg-card hover:bg-card-hover',
				indicator: 'border-icon-muted',
				dot: 'scale-0'
			}
		}
	},
	defaultVariants: { selected: false }
});

// --------------------------------------------------------------- skeleton

export const skeletonVariants = tv({
	base: 'animate-pulse rounded-md bg-secondary/70 motion-reduce:animate-none',
	variants: {
		shape: { line: 'h-3.5', block: '', circle: 'rounded-full', tile: 'size-9 rounded-[0.6rem]' }
	},
	defaultVariants: { shape: 'line' }
});

// ---------------------------------------------------------- linked summary

export const summaryCardVariants = tv({
	slots: {
		root: 'group flex min-w-0 flex-col rounded-xl border border-border bg-card p-3.5 no-underline! focus-ring transition-colors hover:bg-card-hover sm:p-5',
		label:
			'flex min-h-9 items-start justify-between gap-2 text-footnote font-medium text-muted-foreground sm:min-h-0 sm:items-center',
		value: 'mt-2 text-title nums',
		description: 'mt-1 hidden text-footnote text-muted-foreground sm:block'
	}
});
