import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import PrivacyPolicy from './PrivacyPolicy';

describe('PrivacyPolicy', () => {
    it('presents the policy as a public, navigable document', () => {
        render(
            <MemoryRouter>
                <PrivacyPolicy />
            </MemoryRouter>,
        );

        expect(screen.getByRole('heading', { name: /privacy at geoguessme/i })).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Data we collect' })).toHaveAttribute('href', '#data-we-collect');
        expect(screen.getByRole('link', { name: 'Legal bases' })).toHaveAttribute('href', '#legal-bases');
        expect(screen.getByRole('heading', { name: 'Retention, security, and deletion' })).toBeInTheDocument();
        expect(screen.getByText(/camera frames and selected files stay on your device/i)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'privacy@geoguessme.com' })).toHaveAttribute(
            'href',
            'mailto:privacy@geoguessme.com',
        );
    });

    it('explains the legal bases, transfers, and rights behind the processing', () => {
        render(
            <MemoryRouter>
                <PrivacyPolicy />
            </MemoryRouter>,
        );

        expect(screen.getByRole('heading', { name: 'Legal bases' })).toBeInTheDocument();
        const contractBasisCells = screen.getAllByText(/performance of the service/i);
        expect(contractBasisCells.length).toBeGreaterThanOrEqual(3);
        expect(screen.getByText(/standard contractual clauses/i)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'cnil.fr' })).toHaveAttribute('href', 'https://www.cnil.fr');
        expect(screen.getByText(/restrict processing/i)).toBeInTheDocument();
        expect(screen.getByText(/intended for people aged 15 and over/i)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'legal notice' })).toHaveAttribute('href', '/legal');
    });
});
