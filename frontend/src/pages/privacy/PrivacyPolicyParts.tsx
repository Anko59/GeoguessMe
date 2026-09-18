import type { ReactNode } from 'react';

export function PolicySection({
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

export function DetailCard({
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

export function ProviderRow({ detail, title }: { detail: string; title: string }) {
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

export function ChoiceCard({ detail, title }: { detail: string; title: string }) {
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
