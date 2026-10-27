import { describe, expect, it } from 'vitest';
import { cn } from '../utils';
import { listRowVariants } from './variants';

describe('class merging knows the type scale', () => {
	it('keeps a type-scale size next to a text color', () => {
		expect(cn('text-footnote text-muted-foreground')).toBe('text-footnote text-muted-foreground');
		expect(listRowVariants().meta()).toContain('text-footnote');
		expect(listRowVariants().subtitle()).toContain('text-callout');
	});
	it('still resolves real conflicts', () => {
		expect(cn('text-callout', 'text-footnote')).toBe('text-footnote');
		expect(cn('text-sm', 'text-body')).toBe('text-body');
		expect(cn('p-4', 'p-2')).toBe('p-2');
	});
});
