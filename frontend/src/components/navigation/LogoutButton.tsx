import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import './LogoutButton.css';

export default function LogoutButton() {
    const navigate = useNavigate();
    const { logout } = useAuth();

    const handleLogout = async (): Promise<void> => {
        await logout();
        navigate('/');
    };

    return (
        <button className="logout-btn" onClick={handleLogout}>
            <img src="/logout_icon.png" alt="Logout" className="logout-icon" />
            Logout
        </button>
    );
}
