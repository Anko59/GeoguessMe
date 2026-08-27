import { BrowserRouter as Router, Link, Routes, Route } from 'react-router-dom';
import Home from './pages/home/Home';
import Login from './pages/auth/Login';
import Signup from './pages/auth/Signup';
import GroupsList from './pages/groups/GroupsList';
import GroupJoin from './pages/groups/GroupJoin';
import GroupView from './pages/groups/GroupView';
import ProtectedRoute from './components/navigation/ProtectedRoute';
import AuthProvider from './context/AuthProvider';
import ForgotPassword from './pages/auth/ForgotPassword';
import ResetPassword from './pages/auth/ResetPassword';
import VerifyEmail from './pages/auth/VerifyEmail';
import OIDCCallback from './pages/auth/OIDCCallback';
import AccountSettings from './pages/account/AccountSettings';
import ProfilePage from './pages/profile/ProfilePage';
import PwaOnboarding from './components/pwa/PwaOnboarding';
import { usePushBootstrap } from './push/usePushBootstrap';
import { useInviteFragmentCapture } from './hooks/useInviteFragmentCapture';
import { useAuth } from './context/AuthContext';

function AppChrome() {
    const { user } = useAuth();
    usePushBootstrap();
    // Captures #invite=TOKEN fragments into sessionStorage before any auth
    // redirect so the token survives the login/signup hop.
    useInviteFragmentCapture();
    return (
        <div className="app-root">
            {user?.migration_required && (
                <aside className="migration-banner" role="status">
                    <span>This legacy account is read-only until Keycloak is connected.</span>
                    <Link to="/settings">Finish migration</Link>
                </aside>
            )}
            <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/login" element={<Login />} />
                <Route path="/signup" element={<Signup />} />
                <Route path="/migrate-account" element={<Login migrationMode />} />
                <Route path="/forgot-password" element={<ForgotPassword />} />
                <Route path="/reset-password" element={<ResetPassword />} />
                <Route path="/verify-email" element={<VerifyEmail />} />
                <Route path="/auth/oidc/callback" element={<OIDCCallback />} />
                <Route
                    path="/groups"
                    element={
                        <ProtectedRoute>
                            <GroupsList />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/join"
                    element={
                        <ProtectedRoute>
                            <GroupJoin />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/create"
                    element={
                        <ProtectedRoute>
                            <GroupJoin />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/:id"
                    element={
                        <ProtectedRoute>
                            <GroupView />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/profile"
                    element={
                        <ProtectedRoute>
                            <ProfilePage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/profile/:userId"
                    element={
                        <ProtectedRoute>
                            <ProfilePage />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/settings"
                    element={
                        <ProtectedRoute>
                            <AccountSettings />
                        </ProtectedRoute>
                    }
                />
            </Routes>
            <PwaOnboarding />
        </div>
    );
}

function App() {
    return (
        <Router>
            <AuthProvider>
                <AppChrome />
            </AuthProvider>
        </Router>
    );
}

export default App;
