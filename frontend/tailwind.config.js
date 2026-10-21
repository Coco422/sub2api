/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 狗站 / 88dog：暖橙主色，500 起保持白色按钮文字的对比度。
        primary: {
          50: '#fff6ee',
          100: '#fce7d7',
          200: '#f5c6aa',
          300: '#f1a078',
          400: '#e86f3a',
          500: '#bf4b1e',
          600: '#a83d17',
          700: '#893012',
          800: '#6e2c17',
          900: '#582719',
          950: '#32170e'
        },
        gray: {
          50: '#f5f0e7',
          100: '#f0e9df',
          200: '#e4dbcf',
          300: '#d1c4b5',
          400: '#a69585',
          500: '#7d6f62',
          600: '#685a4e',
          700: '#53463c',
          800: '#40352e',
          900: '#29231f',
          950: '#191511'
        },
        // 辅助色 - 暖咖与奶油
        accent: {
          50: '#fffaf4',
          100: '#f8ede1',
          200: '#efd8c2',
          300: '#dfbea0',
          400: '#c89b75',
          500: '#a47853',
          600: '#855e3f',
          700: '#684a34',
          800: '#49372a',
          900: '#31271f',
          950: '#1a1510'
        },
        // 深色模式背景
        dark: {
          50: '#fffcf7',
          100: '#f5f0e7',
          200: '#e7ded2',
          300: '#cec1b2',
          400: '#ad9c8c',
          500: '#897767',
          600: '#6e5e51',
          700: '#4b3f35',
          800: '#342b25',
          900: '#241e19',
          950: '#191511'
        }
      },
      fontFamily: {
        sans: [
          'Avenir Next',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px rgba(232, 111, 58, 0.25)',
        'glow-lg': '0 0 40px rgba(232, 111, 58, 0.35)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #e86f3a 0%, #bf4b1e 100%)',
        'gradient-dark': 'linear-gradient(135deg, #342b25 0%, #241e19 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(232, 111, 58, 0.12) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(251, 146, 60, 0.08) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(232, 111, 58, 0.08) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(232, 111, 58, 0.25)' },
          '100%': { boxShadow: '0 0 30px rgba(232, 111, 58, 0.4)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
