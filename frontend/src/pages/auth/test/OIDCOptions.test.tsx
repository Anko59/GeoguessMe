import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import OIDCOptions from '../OIDCOptions';

vi.mock('../../../platform/endpoints', () => ({
    backendURL: (path: string) => `https://geoguessme.com${path}`,
}));

it('submits native email sign-in to the backend rather than the virtual asset origin', () => {
    render(<OIDCOptions loginPath="/oauth2/start" intent="login" onStart={vi.fn()} socialProviders={['google']} />);
    const form = screen.getByRole('button', { name: 'Continue to password' }).closest('form');
    expect(form).toHaveAttribute('action', 'https://geoguessme.com/oauth2/start');
    expect(form).toHaveAttribute('method', 'get');
    expect(screen.getByRole('link', { name: 'Continue with Google' })).toHaveAttribute(
        'href',
        'https://geoguessme.com/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback&kc_idp_hint=google',
    );
});
