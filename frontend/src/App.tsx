import { BrowserRouter as Router, Routes, Route, useLocation } from 'react-router-dom';
import Home from './pages/home/Home';
import Feed from './pages/feed/Feed';
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
import NotFound from './pages/not-found/NotFound';
import PrivacyPolicy from './pages/privacy/PrivacyPolicy';
import PwaOnboarding from './components/pwa/PwaOnboarding';
import LegalFooter from './components/navigation/LegalFooter';
import { usePushBootstrap } from './push/usePushBootstrap';
import { useInviteFragmentCapture } from './hooks/useInviteFragmentCapture';

function AppChrome() {
    const location = useLocation();
    usePushBootstrap();
    // Captures #invite=TOKEN fragments into sessionStorage before any auth
    // redirect so the token survives the login/signup hop.
    useInviteFragmentCapture();
    return (
        <div className={`app-root${location.pathname === '/' ? ' app-root-home' : ''}`}>
            <Routes>
                <Route path="/" element={<Home />} />
                <Route
                    path="/feed"
                    element={
                        <ProtectedRoute>
                            <Feed />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/feed/:id"
                    element={
                        <ProtectedRoute>
                            <Feed />
                        </ProtectedRoute>
                    }
                />
                <Route path="/login" element={<Login />} />
                <Route path="/signup" element={<Signup />} />
                <Route path="/migrate-account" element={<Login existingAccountMode />} />
                <Route path="/forgot-password" element={<ForgotPassword />} />
                <Route path="/reset-password" element={<ResetPassword />} />
                <Route path="/verify-email" element={<VerifyEmail />} />
                <Route path="/auth/oidc/callback" element={<OIDCCallback />} />
                <Route path="/privacy" element={<PrivacyPolicy />} />
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
                <Route path="*" element={<NotFound />} />
            </Routes>
            <LegalFooter />
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
