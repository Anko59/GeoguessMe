import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import TermsOfUse from './TermsOfUse';
import { contactEmailAddress, operatorContacts, termsUpdatedDate } from './operatorDetails';

describe('TermsOfUse', () => {
    it('presents the rules as a public, navigable document', () => {
        render(
            <MemoryRouter>
                <TermsOfUse />
            </MemoryRouter>,
        );

        expect(screen.getByRole('heading', { name: /fair play/i })).toBeInTheDocument();
        expect(screen.getByText(`Last updated ${termsUpdatedDate}`)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Acceptable use' })).toHaveAttribute('href', '#acceptable-use');
        expect(screen.getByRole('heading', { name: 'Moderation, reporting, and appeals' })).toBeInTheDocument();
    });

    it('states the eligibility, content, and enforcement rules players must follow', () => {
        render(
            <MemoryRouter>
                <TermsOfUse />
            </MemoryRouter>,
        );

        expect(screen.getByText(/be at least 15 years old/i)).toBeInTheDocument();
        expect(screen.getByText(/only upload photos and videos you have the right to share/i)).toBeInTheDocument();
        expect(screen.getByText(/we review reports promptly/i)).toBeInTheDocument();
        expect(screen.getByText(/these terms are governed by french law/i)).toBeInTheDocument();
    });

    it('links the abuse contact and the other legal documents', () => {
        render(
            <MemoryRouter>
                <TermsOfUse />
            </MemoryRouter>,
        );

        const abuseContact = operatorContacts.find((contact) => contact.label.startsWith('Content reports'));
        const abuseAddress = contactEmailAddress(abuseContact!);
        const abuseLinks = screen.getAllByRole('link', { name: abuseAddress });
        expect(abuseLinks.length).toBeGreaterThan(0);
        for (const link of abuseLinks) {
            expect(link).toHaveAttribute('href', `mailto:${abuseAddress}`);
        }
        expect(screen.getAllByRole('link', { name: 'legal notice' })[0]).toHaveAttribute('href', '/legal');
        expect(screen.getAllByRole('link', { name: 'privacy policy' })[0]).toHaveAttribute('href', '/privacy');
    });
});
