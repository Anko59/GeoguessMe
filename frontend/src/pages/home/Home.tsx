import { useContext } from 'react';
import { Link, Navigate } from 'react-router-dom';
import { AuthContext } from '../../context/AuthContext';
import './Home.css';

export default function Home() {
    const auth = useContext(AuthContext);
    if (auth?.isAuthenticated && !auth.loading) return <Navigate to="/groups" replace />;

    return (
        <div className="home-container">
            <div className="home-hero">
                <div className="home-banner-wrapper">
                    <img src="/welcome_banner.png" alt="Welcome Banner" className="home-banner" />
                </div>

                <div className="home-content">
                    <div className="home-welcome-asset">
                        <img src="/welcome_asset.png" alt="Welcome" className="welcome-asset-img" />
                    </div>

                    <h1 className="home-title gradient-text">geoguess.me</h1>
                    <p className="home-tagline">Where Snapchat Meets Geoguessr</p>

                    <div className="home-features">
                        <div className="feature-card">
                            <img src="/camera_icon.png" alt="Camera" className="feature-icon" />
                            <h3>Share Photos</h3>
                            <p>Capture moments with your friends</p>
                        </div>
                        <div className="feature-card">
                            <img src="/globe_icon.png" alt="Globe" className="feature-icon" />
                            <h3>Guess Locations</h3>
                            <p>Challenge your geography skills</p>
                        </div>
                        <div className="feature-card">
                            <img src="/cup_icon.png" alt="Trophy" className="feature-icon" />
                            <h3>Compete</h3>
                            <p>Climb the leaderboard and win</p>
                        </div>
                    </div>

                    <div className="home-actions">
                        <Link to="/signup" className="btn btn-primary btn-large">
                            Get Started - It's Free!
                        </Link>
                        <Link to="/login" className="btn btn-secondary btn-large">
                            Already Playing? Login
                        </Link>
                    </div>

                    <p className="home-footer-text">
                        Join thousands of players worldwide
                    </p>
                </div>
            </div>
        </div>
    );
}
