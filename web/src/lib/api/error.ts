/** An error response from Lor: {"error": {"code", "message"}}. */
export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}
