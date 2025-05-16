import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import ShareDownloadPage from './ShareDownloadPage';

const path = window.location.pathname;
if (path.startsWith('/share/')) {
  createRoot(document.getElementById('root')).render(<ShareDownloadPage />);
} else {
  createRoot(document.getElementById('root')).render(<App />);
} 