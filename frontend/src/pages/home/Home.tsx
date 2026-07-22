import { useContext } from 'react';
import { Link, Navigate, useLocation } from 'react-router-dom';
import { AuthContext } from '../../context/AuthContext';
import './Home.css';

export default function Home() {
    const auth = useContext(AuthContext);
    const location = useLocation();
    const loggingOut = (location.state as { loggingOut?: boolean } | null)?.loggingOut === true;
    if (auth?.isAuthenticated && !auth.loading && !loggingOut) return <Navigate to="/groups" replace />;

    return (
        <div className="home-container">
            <div className="home-hero">
                <div className="home-content">
                    <div className="home-copy">
                        <div className="home-brand">
                            <img src="/logo.png" alt="" className="home-logo" />
                            <span>GeoGuessMe</span>
                        </div>
                        <p className="home-eyebrow">The world is your game board</p>
                        <h1 className="home-title gradient-text">
                            <span className="visually-hidden">geoguess.me — </span>
                            Guess the place. Share the story.
                        </h1>
                        <p className="home-tagline">
                            Turn everyday photos into quick geography challenges with friends.
                        </p>

                        <div className="home-actions">
                            <Link to="/signup" className="btn btn-primary btn-large">
                                Get Started - It's Free!
                            </Link>
                            <Link to="/login" className="btn btn-secondary btn-large">
                                Already Playing? Login
                            </Link>
                        </div>
                    </div>

                    <div className="home-welcome-asset" aria-hidden="true">
                        <img src="/welcome_asset.png" alt="" className="welcome-asset-img" />
                    </div>

                    <div className="home-features">
                        <div className="feature-card">
                            <img src="/camera_icon.png" alt="" className="feature-icon" />
                            <div>
                                <h2>Snap</h2>
                                <p>Share a place from your day.</p>
                            </div>
                        </div>
                        <div className="feature-card">
                            <img src="/globe_icon.png" alt="" className="feature-icon" />
                            <div>
                                <h2>Guess</h2>
                                <p>Find the location on the map.</p>
                            </div>
                        </div>
                        <div className="feature-card">
                            <img src="/cup_icon.png" alt="" className="feature-icon" />
                            <div>
                                <h2>Climb</h2>
                                <p>Earn points with every guess.</p>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
