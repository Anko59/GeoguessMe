import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthContext } from '../../context/AuthContext';
import type { AuthContextValue } from '../../context/AuthContext';
import type { PushSubscriptionState } from '../../push/push';

const mocks = vi.hoisted(() => ({
    isPushSupported: vi.fn(() => false),
    pushPermissionState: vi.fn<() => PushSubscriptionState>(() => 'default'),
    subscribePushNotifications: vi.fn(),
    installable: false,
    installed: false,
    dismissed: false,
    promptInstall: vi.fn(),
    isIosSafari: false,
}));

vi.mock('../../push/push', () => ({
    isPushSupported: mocks.isPushSupported,
    pushPermissionState: mocks.pushPermissionState,
    subscribePushNotifications: mocks.subscribePushNotifications,
}));

vi.mock('./usePwaInstall', async (importOriginal) => {
    const actual = (await importOriginal()) as Record<string, unknown>;
    return {
        ...actual,
        usePwaInstall: () => ({
            installable: mocks.installable,
            installed: mocks.installed,
            dismissed: mocks.dismissed,
            promptInstall: mocks.promptInstall,
            dismiss: vi.fn(),
        }),
        isIosSafari: () => mocks.isIosSafari,
        isStandaloneDisplay: () => mocks.installed,
    };
});

import PwaOnboarding from './PwaOnboarding';

const authBase: AuthContextValue = {
    user: { id: 'u1', username: 'alice', email: 'alice@example.test', avatar: 'avatar.png' },
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => true),
};

function renderWithAuth(auth: Partial<AuthContextValue> = {}) {
    return render(
        <AuthContext.Provider value={{ ...authBase, ...auth }}>
            <PwaOnboarding />
        </AuthContext.Provider>,
    );
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.installable = false;
    mocks.installed = false;
    mocks.dismissed = false;
    mocks.isPushSupported.mockReturnValue(false);
    mocks.pushPermissionState.mockReturnValue('default');
    mocks.isIosSafari = false;
    mocks.subscribePushNotifications.mockReset();
});

describe('PwaOnboarding', () => {
    it('renders nothing when not authenticated', () => {
        const { container } = renderWithAuth({ isAuthenticated: false });
        expect(container.firstChild).toBeNull();
    });

    it('renders nothing when loading', () => {
        const { container } = renderWithAuth({ loading: true });
        expect(container.firstChild).toBeNull();
    });

    it('renders nothing when already installed', () => {
        mocks.installed = true;
        const { container } = renderWithAuth();
        expect(container.firstChild).toBeNull();
    });

    it('renders nothing when dismissed', () => {
        mocks.dismissed = true;
        const { container } = renderWithAuth();
        expect(container.firstChild).toBeNull();
    });

    it('shows install button when installable', () => {
        mocks.installable = true;
        renderWithAuth();
        expect(screen.getByText('Install GeoGuessMe')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Install' })).toBeInTheDocument();
    });

    it('shows iOS Add to Home Screen guide', () => {
        mocks.isIosSafari = true;
        renderWithAuth();
        expect(screen.getByRole('heading', { name: 'Add to Home Screen' })).toBeInTheDocument();
        expect(screen.getByText(/Share/)).toBeInTheDocument();
    });

    it('shows enable-notifications button when push is supported and permission is default', () => {
        mocks.isPushSupported.mockReturnValue(true);
        mocks.pushPermissionState.mockReturnValue('default');
        // Must also be installed for iOS scenario, or non-iOS. Simulate non-iOS.
        mocks.isIosSafari = false;
        renderWithAuth();
        expect(screen.getByRole('button', { name: 'Enable' })).toBeInTheDocument();
    });

    it('shows blocked message when permission is denied', () => {
        mocks.isPushSupported.mockReturnValue(true);
        mocks.pushPermissionState.mockReturnValue('denied');
        renderWithAuth();
        expect(screen.getByText('Blocked in settings')).toBeInTheDocument();
    });

    it('dismiss button is present and removes the banner', async () => {
        mocks.installable = true;
        const { container, rerender } = renderWithAuth();
        expect(screen.getByLabelText('Dismiss install prompt')).toBeInTheDocument();

        // Simulate dismiss via the mock's dismiss function being called.
        const dismissButton = screen.getByLabelText('Dismiss install prompt');
        await userEvent.click(dismissButton);

        // Re-render with dismissed=true to verify the component disappears.
        mocks.dismissed = true;
        rerender(
            <AuthContext.Provider value={authBase}>
                <PwaOnboarding />
            </AuthContext.Provider>,
        );
        expect(container.firstChild).toBeNull();
    });
});
