import { Link } from 'react-router-dom';
import Icon from '../../components/ui/Icon';
import './NotFound.css';

export default function NotFound() {
    return (
        <main className="not-found-page">
            <section className="not-found-card scale-in" aria-labelledby="not-found-title">
                <div className="not-found-visual" aria-hidden="true">
                    <span className="not-found-orbit" />
                    <span className="not-found-logo-frame">
                        <img src="/logo.png" alt="" className="not-found-logo" />
                    </span>
                    <span className="not-found-code gradient-text">404</span>
                </div>

                <p className="not-found-eyebrow">Off the map</p>
                <h1 id="not-found-title">This place isn't on the map</h1>
                <p className="not-found-copy">The link may be outdated, or the address might have a typo.</p>

                <Link to="/" className="btn btn-primary not-found-action">
                    <Icon name="arrow-left" />
                    Back to GeoGuessMe
                </Link>
            </section>
        </main>
    );
}
