import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import LegalNotice from './LegalNotice';
import { contactEmailAddress, hostingProviders, operatorContacts } from './operatorDetails';

describe('LegalNotice', () => {
    it('presents the publication details as a public, navigable document', () => {
        render(
            <MemoryRouter>
                <LegalNotice />
            </MemoryRouter>,
        );

        expect(screen.getByRole('heading', { name: /who is behind/i })).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Publisher' })).toHaveAttribute('href', '#publisher');
        expect(screen.getByRole('heading', { name: 'Hosting' })).toBeInTheDocument();
        expect(screen.getByText(hostingProviders[0].name)).toBeInTheDocument();
        expect(screen.getByText(hostingProviders[0].address)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'OpenStreetMap contributors' })).toHaveAttribute(
            'href',
            'https://www.openstreetmap.org/copyright',
        );
    });

    it('reaches the operator through the published contact addresses', () => {
        render(
            <MemoryRouter>
                <LegalNotice />
            </MemoryRouter>,
        );

        for (const contact of operatorContacts) {
            const address = contactEmailAddress(contact);
            const addressLinks = screen.getAllByRole('link', { name: address });
            expect(addressLinks.length).toBeGreaterThan(0);
            for (const link of addressLinks) {
                expect(link).toHaveAttribute('href', `mailto:${address}`);
            }
        }
    });

    it('cross-links the terms of use and the privacy policy', () => {
        render(
            <MemoryRouter>
                <LegalNotice />
            </MemoryRouter>,
        );

        const termsLinks = screen.getAllByRole('link', { name: 'terms of use' });
        expect(termsLinks.length).toBeGreaterThan(0);
        for (const link of termsLinks) {
            expect(link).toHaveAttribute('href', '/terms');
        }
        expect(screen.getByRole('link', { name: 'privacy policy' })).toHaveAttribute('href', '/privacy');
    });
});
