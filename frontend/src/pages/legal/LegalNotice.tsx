import { Link } from 'react-router-dom';
import LegalDocumentLayout, { LegalDetailCard, LegalSection } from './LegalDocumentLayout';
import {
    contactEmailAddress,
    hostingProviders,
    legalNoticeUpdatedDate,
    operatorContacts,
    operatorIdentity,
    serviceName,
    siteDomain,
} from './operatorDetails';

const navItems = [
    { id: 'publisher', label: 'Publisher' },
    { id: 'publication', label: 'Publication' },
    { id: 'contact', label: 'Contact' },
    { id: 'hosting', label: 'Hosting' },
    { id: 'credits', label: 'Service credits' },
];

const privacyContact = operatorContacts.find((contact) => contact.label.startsWith('Privacy'));
const generalContact = operatorContacts.find((contact) => contact.label.startsWith('General'));
const abuseContact = operatorContacts.find((contact) => contact.label.startsWith('Content reports'));

export default function LegalNotice() {
    return (
        <LegalDocumentLayout
            eyebrow="Legal notice"
            titleLead="Who is behind"
            titleAccent={serviceName}
            lede="The information required for publishing this service: who operates it, who publishes it, how to reach us, and where it runs."
            updatedDate={legalNoticeUpdatedDate}
            navItems={navItems}
        >
            <LegalSection id="publisher" number="01" title="Publisher" kicker="Operator">
                <div className="legal-doc-card-grid">
                    <LegalDetailCard accent="orange" title="Operator">
                        <p>{operatorIdentity.name}</p>
                        {operatorIdentity.legalForm && <p>{operatorIdentity.legalForm}</p>}
                        {operatorIdentity.address && <p>{operatorIdentity.address}</p>}
                        {operatorIdentity.registration && <p>{operatorIdentity.registration}</p>}
                    </LegalDetailCard>
                    <LegalDetailCard accent="blue" title="Website">
                        <p>{`${siteDomain}`}</p>
                        <p>The {serviceName} app and website are published from this domain.</p>
                    </LegalDetailCard>
                </div>
                {!operatorIdentity.identityComplete && (
                    <p className="legal-doc-pending">
                        The complete legal identity of the publisher is being finalized and will be published here
                        before the service is offered commercially. In the meantime, every contact address below reaches
                        the operator directly.
                    </p>
                )}
            </LegalSection>

            <LegalSection id="publication" number="02" title="Publication" kicker="Responsibility">
                <p>
                    <strong>Publication manager:</strong>{' '}
                    {operatorIdentity.publicationManager || `${serviceName} operator`}. The publication manager is
                    responsible for the editorial content published on {siteDomain}.
                </p>
                <p>
                    Content created by players — chat messages, photos, videos, and reactions — is the responsibility of
                    the player who created it, within their private groups, under the{' '}
                    <Link to="/terms">terms of use</Link>.
                </p>
            </LegalSection>

            <LegalSection id="contact" number="03" title="Contact" kicker="Reaching us">
                <ul>
                    {[privacyContact, generalContact, abuseContact].map((contact) =>
                        contact ? (
                            <li key={contact.label}>
                                {contact.label}:{' '}
                                <a href={`mailto:${contactEmailAddress(contact)}`}>{contactEmailAddress(contact)}</a>
                            </li>
                        ) : null,
                    )}
                </ul>
                <p>
                    Security vulnerabilities are handled privately through the responsible-disclosure process described
                    in the project security policy. Content reports and appeals are described in the{' '}
                    <Link to="/terms">terms of use</Link>.
                </p>
            </LegalSection>

            <LegalSection id="hosting" number="04" title="Hosting" kicker="Where the service runs">
                <div className="legal-doc-card-grid">
                    {hostingProviders.map((provider, index) => (
                        <LegalDetailCard
                            key={provider.name}
                            accent={index % 2 === 0 ? 'green' : 'purple'}
                            title={provider.name}
                        >
                            <p>{provider.address}</p>
                            <p>{provider.role}</p>
                        </LegalDetailCard>
                    ))}
                </div>
            </LegalSection>

            <LegalSection id="credits" number="05" title="Service credits" kicker="Data and software">
                <p>
                    Map displays use map data from{' '}
                    <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noreferrer">
                        OpenStreetMap contributors
                    </a>{' '}
                    under the Open Database License. Challenge photos, videos, and messages remain the property of the
                    players who created them, as described in the <Link to="/terms">terms of use</Link>.
                </p>
                <p>
                    GeoGuessMe is an independent project and is not affiliated with the providers listed above. The
                    processing of personal data is described in the <Link to="/privacy">privacy policy</Link>.
                </p>
            </LegalSection>
        </LegalDocumentLayout>
    );
}
