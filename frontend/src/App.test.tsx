import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import Home from './pages/Home';

describe('Home Page', () => {
    it('renders the home page with correct text', () => {
        render(
            <BrowserRouter>
                <Home />
            </BrowserRouter>
        );

        expect(screen.getByText('gueoguess.me')).toBeInTheDocument();
        expect(screen.getByText('Snapchat meets Geoguessr')).toBeInTheDocument();
        expect(screen.getByText('Login')).toBeInTheDocument();
        expect(screen.getByText('Sign Up')).toBeInTheDocument();
    });
});
