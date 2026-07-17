import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Home from './pages/Home';
import Login from './pages/Login';
import Signup from './pages/Signup';
import GroupsList from './pages/GroupsList';
import GroupJoin from './pages/GroupJoin';
import GroupView from './pages/GroupView';
import ProtectedRoute from './components/ProtectedRoute';
import AuthProvider from './context/AuthProvider';
import ForgotPassword from './pages/ForgotPassword';
import ResetPassword from './pages/ResetPassword';
import VerifyEmail from './pages/VerifyEmail';
import AccountSettings from './pages/AccountSettings';

function App() {
  return (
    <Router>
      <AuthProvider>
        <div className="container">
          <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/login" element={<Login />} />
          <Route path="/signup" element={<Signup />} />
          <Route path="/forgot-password" element={<ForgotPassword />} />
          <Route path="/reset-password" element={<ResetPassword />} />
          <Route path="/verify-email" element={<VerifyEmail />} />
          <Route path="/groups" element={<ProtectedRoute><GroupsList /></ProtectedRoute>} />
          <Route path="/group/join" element={<ProtectedRoute><GroupJoin /></ProtectedRoute>} />
          <Route path="/group/create" element={<ProtectedRoute><GroupJoin /></ProtectedRoute>} />
          <Route path="/group/:id" element={<ProtectedRoute><GroupView /></ProtectedRoute>} />
          <Route path="/settings" element={<ProtectedRoute><AccountSettings /></ProtectedRoute>} />
          </Routes>
        </div>
      </AuthProvider>
    </Router>
  );
}

export default App;
