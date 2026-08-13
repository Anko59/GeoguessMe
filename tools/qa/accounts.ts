import path from 'node:path';
import fs from 'node:fs/promises';

export const QA_PASSWORD = 'QaAgentPass123';

export interface Credentials {
    username: string;
    email?: string;
}

export interface QaAccounts {
    owner: Credentials;
    member: Credentials;
    outsider: Credentials;
}

export function accountFile(): string {
    return path.join(process.env.QA_ARTIFACT_DIR || '/tmp/qa-artifacts', '.qa-accounts.json');
}

export async function readAccounts(): Promise<QaAccounts> {
    return JSON.parse(await fs.readFile(accountFile(), 'utf8')) as QaAccounts;
}
