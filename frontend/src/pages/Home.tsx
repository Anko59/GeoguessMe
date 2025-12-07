import { useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import './Home.css';

export default function Home() {
    const navigate = useNavigate();

    useEffect(() => {
        // Check if user is already logged in
        const token = localStorage.getItem('token');
        if (token) {
            // Redirect to groups page if already authenticated
            navigate('/groups');
        }
    }, [navigate]);

    return (
        <div className="home-container">
            <div className="home-content">
                <img src="/welcome_banner.png" alt="Welcome" className="home-banner" />
                <h1 className="home-title gradient-text">geoguess.me</h1>
                <p className="home-tagline">Snapchat meets Geoguessr</p>
                <p className="home-description">
                    📸 Share photos with friends<br />
                    🌍 Guess the locations<br />
                    🏆 Climb the leaderboard
                </p>

                <div className="home-actions">
                    <Link to="/login" className="btn btn-primary">Login</Link>
                    <Link to="/signup" className="btn btn-secondary">Sign Up</Link>
                </div>
            </div>
        </div>
    );
}
