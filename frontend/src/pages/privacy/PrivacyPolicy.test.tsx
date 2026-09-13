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
        expect(screen.getByRole('heading', { name: 'Retention, security, and deletion' })).toBeInTheDocument();
        expect(screen.getByText(/camera frames and selected files stay on your device/i)).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'privacy@geoguessme.com' })).toHaveAttribute(
            'href',
            'mailto:privacy@geoguessme.com',
        );
    });
});
