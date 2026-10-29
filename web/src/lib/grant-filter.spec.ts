import { describe, expect, it } from 'vitest';
import { parseGrantFilter } from './grant-filter';

describe('audit grant filter', () => {
	it.each(['', '  '])('allows an empty filter (%s)', (input) => {
		expect(parseGrantFilter(input)).toEqual({});
	});
	it.each(['7', '#7', '  #7  ', '007'])('accepts a grant number (%s)', (input) => {
		expect(parseGrantFilter(input)).toEqual({ value: 7 });
	});
	it.each(['0', '-7', '7.5', '7.0', '1e3', '0x7', '##7', '#', 'abc', '9007199254740992'])(
		'rejects invalid or imprecise grant numbers (%s)',
		(input) => expect(parseGrantFilter(input).error).toBeTruthy()
	);
});
