import type { Config } from 'tailwindcss';
import defaultTheme from 'tailwindcss/defaultTheme';
import forms from '@tailwindcss/forms';
import typography from '@tailwindcss/typography';

export default {
    content: ['./src/**/*.{html,js,svelte,ts}'],
    theme: {
        extend: {
            colors: {
                // 🛡️ SLA: Centralized Brand Palette
                kari: {
                    teal: '#1BA8A0',        // Primary buttons, highlights, accents
                    'warm-gray': '#8E8F93',  // Secondary UI, borders, disabled states
                    'light-gray': '#F4F5F6', // Backgrounds, cards, panels
                    text: '#1A1A1C',         // Headings, body text, navigation
                },
                // Semantic Mapping for easier utility usage
                brand: {
                    primary: '#1BA8A0',
                    surface: '#F4F5F6',
                    muted: '#8E8F93',
                    dark: '#1A1A1C',
                }
            },
            fontFamily: {
                // 🛡️ Modern System Stack + SF Pro
                sans: [
                    'Inter', // Modern 2026 Standard
                    '-apple-system', 
                    'BlinkMacSystemFont', 
                    '"SF Pro Text"', 
                    '"Segoe UI"', 
                    'Roboto', 
                    ...defaultTheme.fontFamily.sans
                ],
                // 🛡️ Body font for high readability in data-heavy views
                body: ['"IBM Plex Sans"', 'sans-serif'],
                // 🛡️ Monospace font strictly for the Terminal and Code snippets
                mono: ['"IBM Plex Mono"', ...defaultTheme.fontFamily.mono],
            },
            boxShadow: {
                // Custom soft shadow for the Kari Panel "Card" aesthetic
                'kari': '0 4px 20px -2px rgba(26, 26, 28, 0.08)',
            }
        }
    },
    // 🛡️ Zero-Trust UI: Plugins for consistent, secure form styling
    plugins: [
        forms,
        typography
    ]
} satisfies Config;
