import { useNavigate } from 'react-router-dom';
import './LogoutButton.css';

export default function LogoutButton() {
    const navigate = useNavigate();

    const handleLogout = () => {
        // Clear all authentication data
        localStorage.removeItem('token');
        localStorage.removeItem('user');

        // Redirect to home page
        navigate('/');
    };

    return (
        <button className="logout-btn" onClick={handleLogout}>
            <img src="/logout_icon.png" alt="Logout" className="logout-icon" />
            Logout
        </button>
    );
}
