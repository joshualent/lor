// Class-merging config shared by cn() and tailwind-variants: our type scale
// (text-title … text-caption) is a font size, not a text color, so merging
// `text-footnote text-muted-foreground` must keep both.
export const TYPE_SCALE = [
	'title',
	'headline',
	'heading',
	'body',
	'callout',
	'footnote',
	'caption'
] as const;

export const mergeConfig = {
	extend: { classGroups: { 'font-size': [{ text: [...TYPE_SCALE] }] } }
};
