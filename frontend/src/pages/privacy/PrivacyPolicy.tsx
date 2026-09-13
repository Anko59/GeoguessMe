import { Link } from 'react-router-dom';
import type { ReactNode } from 'react';
import './PrivacyPolicy.css';
import './PrivacyPolicyDetails.css';
import './PrivacyPolicyResponsive.css';

const updatedDate = '11 September 2026';

export default function PrivacyPolicy() {
    return (
        <main className="privacy-page">
            <div className="privacy-orbit privacy-orbit-one" aria-hidden="true" />
            <div className="privacy-orbit privacy-orbit-two" aria-hidden="true" />

            <header className="privacy-header">
                <Link to="/" className="privacy-brand" aria-label="GeoGuessMe home">
                    <span className="privacy-brand-mark">
                        <img src="/logo.png" alt="" />
                    </span>
                    <span>GeoGuessMe</span>
                </Link>
                <Link to="/" className="privacy-back-link">
                    Back to the game
                    <span aria-hidden="true">↗</span>
                </Link>
            </header>

            <div className="privacy-layout">
                <aside className="privacy-sidebar" aria-label="Privacy policy navigation">
                    <p className="privacy-sidebar-label">On this page</p>
                    <nav>
                        <a href="#overview">Overview</a>
                        <a href="#data-we-collect">Data we collect</a>
                        <a href="#how-we-use-data">How we use data</a>
                        <a href="#sharing">Sharing and providers</a>
                        <a href="#retention">Retention and deletion</a>
                        <a href="#your-choices">Your choices</a>
                        <a href="#children">Children’s privacy</a>
                        <a href="#changes">Changes</a>
                        <a href="#contact">Contact</a>
                    </nav>
                </aside>

                <article className="privacy-document">
                    <section className="privacy-hero" id="overview" aria-labelledby="privacy-title">
                        <div className="privacy-eyebrow">
                            <span className="privacy-eyebrow-dot" aria-hidden="true" />
                            Trust, mapped clearly
                        </div>
                        <h1 id="privacy-title">
                            Privacy at <span className="gradient-text">GeoGuessMe</span>
                        </h1>
                        <p className="privacy-lede">
                            GeoGuessMe turns photos, places, and friendly competition into shared memories. This policy
                            explains what we collect, why we need it, who can see it, and how you stay in control.
                        </p>
                        <div className="privacy-meta-row">
                            <span>Last updated {updatedDate}</span>
                            <span className="privacy-meta-divider" aria-hidden="true" />
                            <span>Applies to the GeoGuessMe app and website</span>
                        </div>
                    </section>

                    <section className="privacy-highlights" aria-label="Privacy highlights">
                        <div className="privacy-highlight privacy-highlight-orange">
                            <span className="privacy-highlight-number">01</span>
                            <h2>Built for play</h2>
                            <p>We use your information to run games with friends, not to build advertising profiles.</p>
                        </div>
                        <div className="privacy-highlight privacy-highlight-blue">
                            <span className="privacy-highlight-number">02</span>
                            <h2>Local when possible</h2>
                            <p>
                                Camera previews and optional visual effects are processed on your device before you
                                send.
                            </p>
                        </div>
                        <div className="privacy-highlight privacy-highlight-green">
                            <span className="privacy-highlight-number">03</span>
                            <h2>Delete from Settings</h2>
                            <p>You can permanently delete your account and associated gameplay data from the app.</p>
                        </div>
                    </section>

                    <div className="privacy-callout">
                        <span className="privacy-callout-icon" aria-hidden="true">
                            ✦
                        </span>
                        <p>
                            <strong>The short version:</strong> GeoGuessMe stores the account and game information
                            needed to make groups, chat, photo challenges, maps, and scoring work. We keep challenge
                            media for a limited period, restrict game data to the relevant groups, and never sell
                            personal data.
                        </p>
                    </div>

                    <PolicySection id="data-we-collect" number="01" title="Data we collect">
                        <p>
                            We collect information you give us, information created when you play, and limited technical
                            information needed to secure and operate the service. We do not ask for access to
                            information that a feature does not need.
                        </p>
                        <div className="privacy-detail-grid">
                            <DetailCard title="Account and sign-in" accent="orange">
                                <p>
                                    Your username, email address, password hash, email-verification status, avatar
                                    choice, and account timestamps. Passwords are never stored in readable form.
                                </p>
                                <p>
                                    If you use an enabled third-party sign-in option, we receive the provider identifier
                                    and basic profile or contact information needed to create or link your GeoGuessMe
                                    account. We do not receive your provider password.
                                </p>
                            </DetailCard>
                            <DetailCard title="Groups and gameplay" accent="blue">
                                <p>
                                    Group memberships, invite and participation records, chat messages, reactions,
                                    challenge photos or videos, challenge metadata, guesses, scores, and game timing
                                    information.
                                </p>
                                <p>
                                    Group members see content according to the game state and the choices made by the
                                    uploader, including whether a challenge location is hidden until the reveal.
                                </p>
                            </DetailCard>
                            <DetailCard title="Camera, location, and media" accent="green">
                                <p>
                                    When you choose to create a challenge, the app can use your camera, microphone for a
                                    recording, and device location. These permissions are requested by the operating
                                    system and can be denied.
                                </p>
                                <p>
                                    Camera frames and selected files stay on your device until you press Send. Uploaded
                                    images are normalized and EXIF metadata, including embedded GPS coordinates, is
                                    removed.
                                </p>
                            </DetailCard>
                            <DetailCard title="Technical and notification data" accent="purple">
                                <p>
                                    We process session cookies, authentication and one-time-token hashes, WebSocket
                                    ticket data, and security-relevant request metadata. If you enable browser
                                    notifications, we store the encrypted Web Push endpoint and its subscription keys so
                                    notifications can be delivered.
                                </p>
                                <p>
                                    Standard hosting and security systems may also process IP address, browser or device
                                    type, timestamps, and error information to deliver the service and prevent abuse.
                                </p>
                            </DetailCard>
                        </div>
                    </PolicySection>

                    <PolicySection id="how-we-use-data" number="02" title="How we use data">
                        <p>We use the information above for these specific purposes:</p>
                        <ul className="privacy-check-list">
                            <li>
                                Authenticate you, maintain your session, verify your email, and help recover your
                                account.
                            </li>
                            <li>
                                Create groups, deliver invitations, provide chat, and show the game to the right
                                members.
                            </li>
                            <li>
                                Process challenges, protect uploaded media, calculate scores, and show maps and results.
                            </li>
                            <li>Send a notification when you have chosen to enable notifications for a group.</li>
                            <li>
                                Detect abuse, rate-limit requests, troubleshoot failures, and keep the service secure.
                            </li>
                            <li>Respond to privacy, support, account-access, and deletion requests.</li>
                        </ul>
                        <p>
                            GeoGuessMe does not sell personal data, use it for targeted advertising, or use camera
                            frames for facial-recognition identification. Visual effects run on the device and are not a
                            separate biometric-identification service.
                        </p>
                    </PolicySection>

                    <PolicySection id="sharing" number="03" title="Sharing and service providers">
                        <p>
                            We share information only when it is needed to provide GeoGuessMe, when you direct us to
                            share it through a game, or when disclosure is required to protect people, the service, or
                            the law.
                        </p>
                        <div className="privacy-provider-list">
                            <ProviderRow
                                title="Other players"
                                detail="Group members can receive usernames, avatars, messages, challenge media, guesses, and scores that the game makes available to them."
                            />
                            <ProviderRow
                                title="Hosting and storage"
                                detail="Current deployments use Hetzner for server hosting and Cloudflare for network protection, DNS, email routing, and object storage. These providers process application data only as needed to operate the service."
                            />
                            <ProviderRow
                                title="Transactional email"
                                detail="Brevo processes email addresses and message contents for verification, account recovery, and essential service mail."
                            />
                            <ProviderRow
                                title="Maps and push delivery"
                                detail="OpenStreetMap tile infrastructure receives ordinary map-tile requests. If you enable Web Push, the browser’s push service receives encrypted notification delivery requests."
                            />
                            <ProviderRow
                                title="Optional sign-in providers"
                                detail="Keycloak brokers an enabled provider such as Google. Those providers may process sign-in according to their own privacy policies. You choose whether to use that sign-in method."
                            />
                        </div>
                        <p>
                            We may disclose information to comply with a valid legal request, enforce our terms,
                            investigate fraud or abuse, or protect the rights and safety of users and the service. We do
                            not sell or rent personal information to data brokers.
                        </p>
                    </PolicySection>

                    <PolicySection id="retention" number="04" title="Retention, security, and deletion">
                        <h3>How long we keep information</h3>
                        <div className="privacy-retention-table" role="table" aria-label="Data retention summary">
                            <div className="privacy-retention-row privacy-retention-heading" role="row">
                                <span role="columnheader">Information</span>
                                <span role="columnheader">Typical lifecycle</span>
                            </div>
                            <div className="privacy-retention-row" role="row">
                                <span role="cell">Account and group data</span>
                                <span role="cell">
                                    Until you delete the account or the data is no longer needed to provide the service.
                                </span>
                            </div>
                            <div className="privacy-retention-row" role="row">
                                <span role="cell">Challenge media</span>
                                <span role="cell">
                                    The configured media-retention period, currently 30 days by default, then removed
                                    from active object storage.
                                </span>
                            </div>
                            <div className="privacy-retention-row" role="row">
                                <span role="cell">Challenge results</span>
                                <span role="cell">
                                    Scores and limited challenge metadata may remain after the original media is
                                    removed.
                                </span>
                            </div>
                            <div className="privacy-retention-row" role="row">
                                <span role="cell">Authentication material</span>
                                <span role="cell">
                                    Expired sessions, verification tokens, password-reset tokens, and WebSocket tickets
                                    are cleaned up automatically.
                                </span>
                            </div>
                        </div>
                        <h3>How we protect information</h3>
                        <p>
                            We use encrypted transport, secure session cookies, password hashing, authorization checks
                            for group and media access, upload validation, metadata stripping, rate limits, and
                            controlled access to operational systems. No internet service can promise absolute security,
                            so please use a unique password and contact us promptly if you suspect unauthorized access.
                        </p>
                        <h3>What account deletion does</h3>
                        <p>
                            You can delete your account from Settings. We remove the account, sessions, authentication
                            material, group memberships, messages, guesses, challenge views, and associated database
                            records. Uploaded media is queued for deletion from object storage; that final storage step
                            can complete shortly after the account record is removed.
                        </p>
                    </PolicySection>

                    <PolicySection id="your-choices" number="05" title="Your choices and permissions">
                        <div className="privacy-choice-grid">
                            <ChoiceCard
                                title="Review or correct"
                                detail="Update your username, recovery email, and avatar in Settings. Contact us if you need help with a data request."
                            />
                            <ChoiceCard
                                title="Control permissions"
                                detail="Deny camera, microphone, location, or notification permission in your device or browser settings. Features that need a denied permission may not work."
                            />
                            <ChoiceCard
                                title="Leave a group"
                                detail="Group membership and access can be managed through the group controls. Other players may retain messages or scores that were already shared until the applicable account or group data is deleted."
                            />
                            <ChoiceCard
                                title="Delete your account"
                                detail="Use the in-app deletion flow, or email privacy@geoguessme.com with enough information for us to locate the account safely."
                            />
                        </div>
                    </PolicySection>

                    <PolicySection id="children" number="06" title="Children’s privacy">
                        <p>
                            GeoGuessMe is not directed to children under 13, and we do not knowingly collect personal
                            information from children under 13. If you believe a child has provided personal
                            information, please contact us so we can investigate and remove it where appropriate. If a
                            higher minimum age applies where you live, follow that requirement.
                        </p>
                    </PolicySection>

                    <PolicySection id="changes" number="07" title="Changes to this policy">
                        <p>
                            We may update this policy when GeoGuessMe changes or when privacy requirements evolve. We
                            will update the date at the top of this page and, when a change is material, provide a
                            clearer notice in the app or through an appropriate service message.
                        </p>
                    </PolicySection>

                    <section
                        id="contact"
                        className="privacy-section privacy-contact-section"
                        aria-labelledby="contact-title"
                    >
                        <div className="privacy-section-heading">
                            <span className="privacy-section-number">08</span>
                            <div>
                                <p className="privacy-section-kicker">Questions or requests</p>
                                <h2 id="contact-title">Let’s keep the map clear.</h2>
                            </div>
                        </div>
                        <p>
                            For privacy questions, access or correction requests, or account deletion support, email{' '}
                            <a href="mailto:privacy@geoguessme.com">privacy@geoguessme.com</a>. Please do not send
                            passwords, authentication codes, or other sensitive credentials by email.
                        </p>
                        <div className="privacy-contact-actions">
                            <a className="btn btn-primary" href="mailto:privacy@geoguessme.com">
                                Email the privacy team
                                <span aria-hidden="true">→</span>
                            </a>
                            <Link className="btn btn-outline" to="/">
                                Return to GeoGuessMe
                            </Link>
                        </div>
                    </section>

                    <p className="privacy-document-note">GeoGuessMe · Privacy policy · Last updated {updatedDate}</p>
                </article>
            </div>
        </main>
    );
}

function PolicySection({
    children,
    id,
    number,
    title,
}: {
    children: ReactNode;
    id: string;
    number: string;
    title: string;
}) {
    return (
        <section id={id} className="privacy-section" aria-labelledby={`${id}-title`}>
            <div className="privacy-section-heading">
                <span className="privacy-section-number">{number}</span>
                <div>
                    <p className="privacy-section-kicker">GeoGuessMe policy</p>
                    <h2 id={`${id}-title`}>{title}</h2>
                </div>
            </div>
            <div className="privacy-section-content">{children}</div>
        </section>
    );
}

function DetailCard({
    accent,
    children,
    title,
}: {
    accent: 'blue' | 'green' | 'orange' | 'purple';
    children: ReactNode;
    title: string;
}) {
    return (
        <div className={`privacy-detail-card privacy-detail-card-${accent}`}>
            <span className="privacy-detail-marker" aria-hidden="true" />
            <h3>{title}</h3>
            {children}
        </div>
    );
}

function ProviderRow({ detail, title }: { detail: string; title: string }) {
    return (
        <div className="privacy-provider-row">
            <span className="privacy-provider-arrow" aria-hidden="true">
                ↗
            </span>
            <div>
                <h3>{title}</h3>
                <p>{detail}</p>
            </div>
        </div>
    );
}

function ChoiceCard({ detail, title }: { detail: string; title: string }) {
    return (
        <div className="privacy-choice-card">
            <span className="privacy-choice-check" aria-hidden="true">
                ✓
            </span>
            <div>
                <h3>{title}</h3>
                <p>{detail}</p>
            </div>
        </div>
    );
}
