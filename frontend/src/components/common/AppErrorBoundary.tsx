import { Component, type ReactNode } from 'react';

type Props = { children: ReactNode };
type State = { failed: boolean };

/** A render exception must not leave a native WebView looking like an empty page. */
export default class AppErrorBoundary extends Component<Props, State> {
    state: State = { failed: false };

    static getDerivedStateFromError(): State {
        return { failed: true };
    }

    componentDidCatch(): void {
        // Do not transmit or print exception text: API URLs can contain secrets.
        console.error('GeoGuessMe could not render the current screen');
    }

    render(): ReactNode {
        if (this.state.failed) {
            return (
                <main
                    role="alert"
                    style={{ maxWidth: '32rem', margin: '15vh auto', padding: '2rem', textAlign: 'center' }}
                >
                    <h1>Something went wrong</h1>
                    <p>
                        This screen could not load. Please reopen the app. If it keeps happening, report your app
                        version.
                    </p>
                    <button type="button" onClick={() => window.location.reload()}>
                        Reload app
                    </button>
                </main>
            );
        }
        return this.props.children;
    }
}
