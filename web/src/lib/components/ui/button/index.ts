import Root, { type ButtonProps } from './button.svelte';
// Variants live with the rest of the design system.
import { buttonVariants, type ButtonSize, type ButtonVariant } from '#lib/design/variants.js';

export {
	Root,
	type ButtonProps as Props,
	//
	Root as Button,
	buttonVariants,
	type ButtonProps,
	type ButtonSize,
	type ButtonVariant
};
