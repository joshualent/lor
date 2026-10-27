import { describe, expect, it } from 'vitest';
import { nextIndex } from './roving';

describe('nextIndex', () => {
	it('wraps with the arrow keys', () => {
		expect(nextIndex('ArrowRight', 2, 3)).toBe(0);
		expect(nextIndex('ArrowLeft', 0, 3)).toBe(2);
		expect(nextIndex('ArrowDown', 0, 3)).toBe(1);
	});
	it('jumps with Home and End and ignores other keys', () => {
		expect(nextIndex('Home', 2, 3)).toBe(0);
		expect(nextIndex('End', 0, 3)).toBe(2);
		expect(nextIndex('a', 0, 3)).toBeNull();
		expect(nextIndex('ArrowRight', 0, 0)).toBeNull();
	});
});
