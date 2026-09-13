import { Link } from 'react-router-dom';
import './LegalFooter.css';

export default function LegalFooter() {
    return (
        <footer className="legal-footer" aria-label="Legal information">
            <div className="legal-footer-inner">
                <span>GeoGuessMe · Play the world together</span>
                <nav className="legal-footer-links" aria-label="Legal links">
                    <Link to="/privacy">Privacy policy</Link>
                    <a href="mailto:privacy@geoguessme.com">Privacy contact</a>
                </nav>
            </div>
        </footer>
    );
}
