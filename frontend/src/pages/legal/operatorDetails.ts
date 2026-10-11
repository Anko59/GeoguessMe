/**
 * Single source of truth for the public legal pages (legal notice and terms of
 * use). Operator identity details stay out of Git history on purpose, so the
 * legal entity below is completed by the operator before public launch.
 * TODO(#302): fill in the legal identity and confirm the abuse mailbox.
 */

export interface OperatorContact {
    /** Public label shown next to the address, for example "Privacy". */
    label: string;
    /** Destination mailbox, without the domain. */
    mailbox: string;
}

export interface HostingProvider {
    name: string;
    address: string;
    role: string;
}

export const serviceName = 'GeoGuessMe';

export const siteDomain = 'geoguessme.com';

/**
 * Identity of the publisher. Every field must be filled before the service is
 * offered professionally; the legal-notice page renders an explicit notice
 * while `identityComplete` is false.
 */
export const operatorIdentity = {
    identityComplete: false,
    name: serviceName,
    legalForm: '',
    address: '',
    registration: '',
    publicationManager: '',
    phone: '',
};

export const operatorContacts: OperatorContact[] = [
    { label: 'Privacy and data protection', mailbox: 'privacy' },
    { label: 'General and legal questions', mailbox: 'privacy' },
    { label: 'Content reports and abuse', mailbox: 'privacy' },
];

export function contactEmailAddress(contact: OperatorContact): string {
    return `${contact.mailbox}@${siteDomain}`;
}

export const hostingProviders: HostingProvider[] = [
    {
        // Cloud contracting entity and postal address, verified against
        // https://www.hetzner.com/legal/cloud-server/ and /legal/legal-notice/.
        name: 'Hetzner Online GmbH',
        address: 'Industriestr. 25, 91710 Gunzenhausen, Germany',
        role: 'Application and database servers',
    },
    {
        name: 'Cloudflare, Inc.',
        address: '101 Townsend St, San Francisco, CA 94107, USA',
        role: 'Network edge, DNS, and email routing',
    },
];

export const termsUpdatedDate = '17 September 2026';

export const legalNoticeUpdatedDate = '3 October 2026';
