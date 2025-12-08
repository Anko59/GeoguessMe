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

        expect(screen.getByText('geoguess.me')).toBeInTheDocument();
        expect(screen.getByText('Where Snapchat Meets Geoguessr')).toBeInTheDocument();
        expect(screen.getByText('Already Playing? Login')).toBeInTheDocument();
        expect(screen.getByText("Get Started - It's Free!")).toBeInTheDocument();
    });
});
