import { Link } from 'react-router-dom';
import './Home.css';

export default function Home() {
    return (
        <div className="home-container">
            <div className="home-content">
                <img src="/welcome_banner.png" alt="Welcome" className="home-banner" />
                <h1 className="home-title gradient-text">gueoguess.me</h1>
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
