type OIDCOptionsProps = {
    loginPath: string;
    intent: 'login' | 'signup';
    onStart: () => void;
    socialProviders: readonly ProviderAlias[];
};

const callbackPath = '/auth/oidc/callback';

const providers = [
    { alias: 'google', label: 'Google' },
    { alias: 'apple', label: 'Apple' },
    { alias: 'github', label: 'GitHub' },
] as const;

type ProviderAlias = (typeof providers)[number]['alias'];

function ProviderLogo({ provider }: { provider: ProviderAlias }) {
    if (provider === 'google') {
        return (
            <svg className="auth-provider-logo auth-provider-logo-google" viewBox="0 0 18 18" aria-hidden="true">
                <path
                    fill="#4285f4"
                    d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.482h4.844a4.14 4.14 0 0 1-1.797 2.715v2.258h2.909c1.702-1.567 2.684-3.875 2.684-6.614Z"
                />
                <path
                    fill="#34a853"
                    d="M9 18c2.43 0 4.468-.806 5.956-2.181l-2.909-2.258c-.806.54-1.836.859-3.047.859-2.344 0-4.328-1.585-5.037-3.714H.956v2.332A9 9 0 0 0 9 18Z"
                />
                <path
                    fill="#fbbc05"
                    d="M3.963 10.706A5.42 5.42 0 0 1 3.681 9c0-.592.102-1.168.282-1.706V4.962H.956A9 9 0 0 0 0 9c0 1.452.347 2.826.956 4.038l3.007-2.332Z"
                />
                <path
                    fill="#ea4335"
                    d="M9 3.58c1.322 0 2.508.454 3.441 1.346l2.581-2.581C13.464.892 11.43 0 9 0A9 9 0 0 0 .956 4.962l3.007 2.332C4.672 5.165 6.656 3.58 9 3.58Z"
                />
            </svg>
        );
    }

    if (provider === 'apple') {
        return (
            <svg className="auth-provider-logo auth-provider-logo-apple" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M17.05 20.28c-.98.95-2.05.8-3.08.35-1.09-.46-2.09-.48-3.24 0-1.44.62-2.2.44-3.06-.35-4.88-5.03-4.16-12.69 1.38-12.97 1.35.07 2.29.74 3.08.8 1.18-.24 2.31-.93 3.57-.84 1.51.12 2.65.72 3.4 1.8-3.12 1.87-2.38 5.98.48 7.13-.57 1.5-1.31 2.99-2.53 4.08ZM12.03 7.25C11.88 5.02 13.69 3.18 15.77 3c.29 2.58-2.34 4.5-3.74 4.25Z" />
            </svg>
        );
    }

    return (
        <svg className="auth-provider-logo auth-provider-logo-github" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M12 2C6.477 2 2 6.477 2 12c0 4.419 2.865 8.166 6.839 9.489.5.092.682-.217.682-.483 0-.237-.009-1.025-.014-1.862-2.782.604-3.369-1.342-3.369-1.342-.455-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.091-.646.349-1.087.635-1.337-2.221-.253-4.555-1.111-4.555-4.943 0-1.092.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0 1 12 7.669a9.59 9.59 0 0 1 2.504.337c1.909-1.294 2.748-1.025 2.748-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.591 1.028 2.683 0 3.842-2.337 4.687-4.566 4.935.359.309.679.92.679 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.003 10.003 0 0 0 22 12c0-5.523-4.477-10-10-10Z" />
        </svg>
    );
}

function startURL(loginPath: string, parameters: Record<string, string>): string {
    const query = new URLSearchParams({ rd: callbackPath, ...parameters });
    return `${loginPath}?${query.toString()}`;
}

export default function OIDCOptions({ loginPath, intent, onStart, socialProviders }: OIDCOptionsProps) {
    const signup = intent === 'signup';

    return (
        <>
            <div className="auth-social-grid" aria-label={signup ? 'Social signup options' : 'Social login options'}>
                {providers.map((provider) => {
                    const available = socialProviders.includes(provider.alias);
                    const content = (
                        <>
                            <ProviderLogo provider={provider.alias} />
                            <span>
                                {signup ? 'Sign up' : 'Continue'} with {provider.label}
                            </span>
                        </>
                    );
                    return available ? (
                        <a
                            className={`btn btn-social btn-social-${provider.alias}`}
                            href={startURL(loginPath, { kc_idp_hint: provider.alias })}
                            onClick={onStart}
                            key={provider.alias}
                        >
                            {content}
                        </a>
                    ) : (
                        <button
                            type="button"
                            className={`btn btn-social btn-social-${provider.alias}`}
                            disabled
                            title={`${provider.label} is not configured in this environment`}
                            key={provider.alias}
                        >
                            {content}
                        </button>
                    );
                })}
            </div>

            <div className="auth-divider">or use email</div>

            <form action={loginPath} method="get" className="auth-form auth-oidc-form" onSubmit={onStart}>
                <input type="hidden" name="rd" value={callbackPath} />
                {signup && <input type="hidden" name="prompt" value="create" />}
                <label htmlFor={`${intent}-email`}>Email address</label>
                <input
                    id={`${intent}-email`}
                    name="login_hint"
                    type="email"
                    placeholder="you@example.com"
                    required
                    autoComplete="email"
                />
                <button type="submit" className="btn btn-primary">
                    {signup ? 'Continue to create account' : 'Continue to password'}
                </button>
            </form>

            <p className="auth-provider-note">
                {socialProviders.length === 0 && 'Social sign-in is not configured in this environment. '}
                Your password is entered only on GeoGuessMe ID. Two-factor authentication and passkeys stay optional.
            </p>
        </>
    );
}
