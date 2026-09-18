import { Link } from 'react-router-dom';
import type { ReactNode } from 'react';
import { serviceName } from './operatorDetails';
import './legalDocument.css';

export interface LegalNavItem {
    id: string;
    label: string;
}

interface LegalDocumentLayoutProps {
    eyebrow: string;
    titleLead: string;
    titleAccent: string;
    lede: string;
    updatedDate: string;
    navItems: LegalNavItem[];
    children: ReactNode;
}

/**
 * Shared chrome for the public legal pages. It mirrors the privacy-policy
 * presentation (brand header, sticky section navigation, numbered sections) so
 * every legal document reads as part of one trust centre.
 */
export default function LegalDocumentLayout({
    eyebrow,
    titleLead,
    titleAccent,
    lede,
    updatedDate,
    navItems,
    children,
}: LegalDocumentLayoutProps) {
    return (
        <main className="legal-doc-page">
            <div className="legal-doc-orbit legal-doc-orbit-one" aria-hidden="true" />
            <div className="legal-doc-orbit legal-doc-orbit-two" aria-hidden="true" />

            <header className="legal-doc-header">
                <Link to="/" className="legal-doc-brand" aria-label={`${serviceName} home`}>
                    <span className="legal-doc-brand-mark">
                        <img src="/logo.png" alt="" />
                    </span>
                    <span>{serviceName}</span>
                </Link>
                <Link to="/" className="legal-doc-back-link">
                    Back to the game
                    <span aria-hidden="true">↗</span>
                </Link>
            </header>

            <div className="legal-doc-layout">
                <aside className="legal-doc-sidebar" aria-label="Document navigation">
                    <p className="legal-doc-sidebar-label">On this page</p>
                    <nav>
                        {navItems.map((item) => (
                            <a key={item.id} href={`#${item.id}`}>
                                {item.label}
                            </a>
                        ))}
                    </nav>
                </aside>

                <article className="legal-doc-document">
                    <section className="legal-doc-hero" aria-labelledby="legal-doc-title">
                        <div className="legal-doc-eyebrow">
                            <span className="legal-doc-eyebrow-dot" aria-hidden="true" />
                            {eyebrow}
                        </div>
                        <h1 id="legal-doc-title">
                            {titleLead} <span className="gradient-text">{titleAccent}</span>
                        </h1>
                        <p className="legal-doc-lede">{lede}</p>
                        <div className="legal-doc-meta-row">
                            <span>Last updated {updatedDate}</span>
                            <span className="legal-doc-meta-divider" aria-hidden="true" />
                            <span>Applies to the {serviceName} app and website</span>
                        </div>
                    </section>
                    {children}
                    <p className="legal-doc-note">
                        {serviceName} · Legal document · Last updated {updatedDate}
                    </p>
                </article>
            </div>
        </main>
    );
}

export function LegalSection({
    children,
    id,
    number,
    kicker = 'Terms of use',
    title,
}: {
    children: ReactNode;
    id: string;
    number: string;
    kicker?: string;
    title: string;
}) {
    return (
        <section id={id} className="legal-doc-section" aria-labelledby={`${id}-title`}>
            <div className="legal-doc-section-heading">
                <span className="legal-doc-section-number">{number}</span>
                <div>
                    <p className="legal-doc-section-kicker">{kicker}</p>
                    <h2 id={`${id}-title`}>{title}</h2>
                </div>
            </div>
            <div className="legal-doc-section-content">{children}</div>
        </section>
    );
}

export function LegalDetailCard({
    accent,
    title,
    children,
}: {
    accent: 'blue' | 'green' | 'orange' | 'purple';
    title: string;
    children: ReactNode;
}) {
    return (
        <div className={`legal-doc-card legal-doc-card-${accent}`}>
            <span className="legal-doc-card-marker" aria-hidden="true" />
            <h3>{title}</h3>
            {children}
        </div>
    );
}

export function LegalCallout({ children }: { children: ReactNode }) {
    return (
        <div className="legal-doc-callout">
            <span className="legal-doc-callout-icon" aria-hidden="true">
                ✦
            </span>
            <p>{children}</p>
        </div>
    );
}
