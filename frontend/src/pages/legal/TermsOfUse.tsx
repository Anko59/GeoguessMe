import { Link } from 'react-router-dom';
import LegalDocumentLayout, { LegalCallout, LegalDetailCard, LegalSection } from './LegalDocumentLayout';
import { contactEmailAddress, operatorContacts, serviceName, termsUpdatedDate } from './operatorDetails';

const navItems = [
    { id: 'about', label: 'About these terms' },
    { id: 'eligibility', label: 'Eligibility and accounts' },
    { id: 'acceptable-use', label: 'Acceptable use' },
    { id: 'your-content', label: 'Your content' },
    { id: 'moderation', label: 'Moderation and reports' },
    { id: 'service-changes', label: 'Availability and changes' },
    { id: 'liability', label: 'Liability' },
    { id: 'law', label: 'Law and disputes' },
    { id: 'contact', label: 'Contact' },
];

const abuseContact = operatorContacts.find((contact) => contact.label.startsWith('Content reports'));
const generalContact = operatorContacts.find((contact) => contact.label.startsWith('General'));

export default function TermsOfUse() {
    return (
        <LegalDocumentLayout
            eyebrow="Terms of use"
            titleLead="Fair play,"
            titleAccent="clear rules"
            lede="These terms are the agreement between you and the GeoGuessMe operator. They explain what you can expect from the game and what we expect from you, so every group stays a safe place to play."
            updatedDate={termsUpdatedDate}
            navItems={navItems}
        >
            <LegalCallout>
                <strong>The short version:</strong> play with people who agreed to be photographed or filmed, keep chat
                friendly and legal, report anything that crosses the line, and remember that challenge media is
                short-lived by design. Breaking these rules can end with content removal or account suspension.
            </LegalCallout>

            <LegalSection id="about" number="01" title="About these terms">
                <p>
                    GeoGuessMe is a shared photo and video challenge game for private groups. These terms of use form a
                    binding agreement between you and the operator of {serviceName} for your use of the app and the
                    website. By creating an account or using the service, you accept these terms.
                </p>
                <p>
                    If you do not agree with these terms, do not create an account and stop using the service. The
                    privacy policy, available at <Link to="/privacy">geoguessme.com/privacy</Link>, explains how
                    personal data is processed and forms part of this agreement.
                </p>
            </LegalSection>

            <LegalSection id="eligibility" number="02" title="Eligibility and accounts">
                <p>To keep an account, you must:</p>
                <ul>
                    <li>
                        be at least 15 years old, or the minimum digital-consent age in your country if it is higher;
                    </li>
                    <li>provide accurate account information and keep your password secret;</li>
                    <li>use the service for personal, non-commercial play;</li>
                    <li>not share your account and not create accounts to evade a previous suspension.</li>
                </ul>
                <p>
                    You can stop using the service at any time and delete your account from Settings. We may suspend or
                    terminate an account that breaks these terms, that is used for unlawful activity, or that endangers
                    other players. Where practicable we will tell you why, and you can answer through the contact
                    address in the moderation section.
                </p>
                <p>
                    Group organisers decide who can join their groups. We do not arbitrate personal disagreements
                    between players beyond the moderation rules below.
                </p>
            </LegalSection>

            <LegalSection id="acceptable-use" number="03" title="Acceptable use">
                <p>
                    GeoGuessMe groups are private spaces between people who know each other. You must not use the
                    service to create, upload, send, or share:
                </p>
                <ul>
                    <li>content that is illegal where you live, including content that incites violence or hatred;</li>
                    <li>sexual content, and absolutely no sexualised content involving minors;</li>
                    <li>harassment, threats, humiliating material, or coordinated abuse of another player;</li>
                    <li>
                        images or recordings of people who did not agree to be captured, or content that exposes
                        someone’s identity, home, or location against their will;
                    </li>
                    <li>material you do not have the rights to use, including unlicensed music or films;</li>
                    <li>
                        spam, scams, phishing, malware, or automated bulk requests, including bulk map-tile downloads;
                    </li>
                    <li>content that impersonates another person or the GeoGuessMe team.</li>
                </ul>
                <p>
                    You must also not attack the service: no scanning, overloading, bypassing authentication or rate
                    limits, or accessing groups and media you are not a member of.
                </p>
            </LegalSection>

            <LegalSection id="your-content" number="04" title="Your content and licences">
                <p>
                    You keep ownership of everything you create on GeoGuessMe: chat messages, challenge photos and
                    videos, avatars, and reactions — together, your content.
                </p>
                <p>You give the operator a limited, worldwide, royalty-free licence to use your content only to:</p>
                <ul>
                    <li>store, process, and transmit it as needed to run the game you joined;</li>
                    <li>show it to the members of the relevant group according to the game state;</li>
                    <li>normalize media, strip metadata, generate the previews the game needs, and back it up;</li>
                    <li>review and act on content that is reported to us.</li>
                </ul>
                <p>
                    That licence ends when your content is deleted, except for backups that expire automatically or
                    content another player was already shown as part of a finished challenge.
                </p>
                <div className="legal-doc-card-grid">
                    <LegalDetailCard accent="blue" title="You are responsible for your content">
                        <p>
                            Only upload photos and videos you have the right to share, and only show people who agreed
                            to appear. Bystanders did not agree to join the game — frame your challenges so other people
                            are not identifiable unless they consented.
                        </p>
                    </LegalDetailCard>
                    <LegalDetailCard accent="green" title="Short-lived by design">
                        <p>
                            Challenge media is deleted after the retention period published in the privacy policy,
                            currently 30 days by default. Scores and limited challenge metadata can remain after the
                            media is gone.
                        </p>
                    </LegalDetailCard>
                </div>
            </LegalSection>

            <LegalSection id="moderation" number="05" title="Moderation, reporting, and appeals">
                <p>
                    We do not monitor private group content in real time, but we act on everything that is reported to
                    us. If you see content or behaviour that breaks these terms or the law, report it:
                </p>
                <ul>
                    <li>to the group organiser, who can remove members from their group;</li>
                    <li>
                        to us by email at{' '}
                        {abuseContact ? (
                            <a href={`mailto:${contactEmailAddress(abuseContact)}`}>
                                {contactEmailAddress(abuseContact)}
                            </a>
                        ) : (
                            'our published contact address'
                        )}
                        , describing what happened and where in the app it happened;
                    </li>
                    <li>
                        to your local authorities when someone is in immediate danger — then tell us so we can preserve
                        the evidence we hold.
                    </li>
                </ul>
                <p>
                    We review reports promptly and can warn users, remove content, suspend accounts, and contact
                    authorities where the law requires it. If we act against your account or content, you can appeal by
                    answering the decision email; a person who was not involved in the original decision reviews every
                    appeal.
                </p>
            </LegalSection>

            <LegalSection id="service-changes" number="06" title="Availability and changes">
                <p>
                    We work hard to keep GeoGuessMe available and enjoyable, but the service is provided with reasonable
                    skill and care, without a promise that it will always be uninterrupted or error-free. Challenge
                    media is short-lived by design and is not a storage or backup service.
                </p>
                <p>
                    We may add, change, or remove features, and we may update these terms when the service or the law
                    changes. We will publish the updated terms on this page with a new date, and give reasonable advance
                    notice in the app for material changes. Continuing to use the service after new terms take effect
                    means you accept them; if you do not, you can delete your account.
                </p>
            </LegalSection>

            <LegalSection id="liability" number="07" title="Liability">
                <p>
                    To the fullest extent permitted by law, the operator is not liable for indirect or consequential
                    loss, including lost data that you did not keep copies of, lost profits, or harm caused by other
                    players’ content, except where we failed to meet our obligations with intent or gross negligence, or
                    where liability cannot be limited under mandatory law.
                </p>
                <p>
                    Nothing in these terms excludes liability that cannot be excluded under the law that applies to you,
                    including the statutory rights of consumers. Nothing in these terms limits your right to complain to
                    a data-protection or consumer-protection authority.
                </p>
            </LegalSection>

            <LegalSection id="law" number="08" title="Governing law and disputes">
                <p>
                    These terms are governed by French law. If you live in the European Union, you also benefit from the
                    mandatory protections of the law of your country of residence, and you may bring proceedings in the
                    courts of that country. Before starting formal proceedings, please contact us — most disputes can be
                    resolved by talking.
                </p>
                <p>
                    If a court decides that one part of these terms is unenforceable, the remaining parts stay in force.
                </p>
            </LegalSection>

            <LegalSection id="contact" number="09" title="Contact" kicker="Questions about these terms">
                <p>
                    Questions about these terms, content reports, and account issues can be sent to{' '}
                    {generalContact ? (
                        <a href={`mailto:${contactEmailAddress(generalContact)}`}>
                            {contactEmailAddress(generalContact)}
                        </a>
                    ) : (
                        'our published contact address'
                    )}
                    . The legal notice page identifies the operator and hosting providers, and the privacy policy
                    explains how personal data is handled.
                </p>
                <div className="legal-doc-card-grid">
                    <LegalDetailCard accent="orange" title="Legal notice">
                        <p>
                            Operator identity, publication details, and hosting providers are published at the{' '}
                            <Link to="/legal">legal notice</Link> page.
                        </p>
                    </LegalDetailCard>
                    <LegalDetailCard accent="purple" title="Privacy policy">
                        <p>
                            What we collect, why, and for how long is described in the{' '}
                            <Link to="/privacy">privacy policy</Link>.
                        </p>
                    </LegalDetailCard>
                </div>
            </LegalSection>
        </LegalDocumentLayout>
    );
}
