import { join } from 'node:path';

/** Saved session for the logged-in admin, written by auth.setup.ts. */
export const authFile = join(__dirname, '..', '.auth', 'admin.json');
