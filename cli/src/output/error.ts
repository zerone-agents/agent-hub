export const EXIT = {
  SUCCESS: 0,
  BAD_REQUEST: 2,
  UNAUTHORIZED: 3,
  FORBIDDEN: 4,
  NOT_FOUND: 5,
  CONFLICT: 6,
  VALIDATION: 7,
  SERVER_ERROR: 8,
  NETWORK_ERROR: 9,
} as const;

export function exitFromHttpStatus(status: number): number {
  if (status >= 500) return EXIT.SERVER_ERROR;
  switch (status) {
    case 400: return EXIT.BAD_REQUEST;
    case 401: return EXIT.UNAUTHORIZED;
    case 403: return EXIT.FORBIDDEN;
    case 404: return EXIT.NOT_FOUND;
    case 409: return EXIT.CONFLICT;
    case 422: return EXIT.VALIDATION;
    default: return EXIT.SERVER_ERROR;
  }
}
