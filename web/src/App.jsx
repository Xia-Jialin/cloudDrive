import React, { useState } from 'react';
import axios from 'axios';
import './App.css';
import FileListPage from './FileListPage';

function passwordComplexityCheck(password) {
  let hasUpper = /[A-Z]/.test(password);
  let hasLower = /[a-z]/.test(password);
  let hasDigit = /[0-9]/.test(password);
  let hasSpecial = /[!@#$%^&*()_+\-=[\]{};':"\\|,.<>/?`~]/.test(password);
  let count = [hasUpper, hasLower, hasDigit, hasSpecial].filter(Boolean).length;
  return password.length >= 6 && password.length <= 32 && count >= 3;
}

function Register({ onSwitch }) {
  const [form, setForm] = useState({ username: '', password: '' });
  const [msg, setMsg] = useState('');
  const [pwdTip, setPwdTip] = useState('');
  const handleChange = e => {
    setForm(f => ({ ...f, [e.target.name]: e.target.value }));
    if (e.target.name === 'password') {
      const pwd = e.target.value;
      if (!pwd) {
        setPwdTip('密码不能为空');
      } else if (pwd.length < 6 || pwd.length > 32) {
        setPwdTip('密码长度需为6-32位');
      } else {
        let hasUpper = /[A-Z]/.test(pwd);
        let hasLower = /[a-z]/.test(pwd);
        let hasDigit = /[0-9]/.test(pwd);
        let hasSpecial = /[!@#$%^&*()_+\-=[\]{};':"\\|,.<>/?`~]/.test(pwd);
        let count = [hasUpper, hasLower, hasDigit, hasSpecial].filter(Boolean).length;
        if (count < 3) {
          setPwdTip('密码需包含大写字母、小写字母、数字、特殊字符中的至少三种');
        } else {
          setPwdTip('密码复杂度合规');
        }
      }
    }
  };
  const handleSubmit = async e => {
    e.preventDefault();
    setMsg('');
    if (!passwordComplexityCheck(form.password)) {
      setMsg('密码不符合复杂度要求：需包含大写字母、小写字母、数字、特殊字符中的至少三种，长度6-32位');
      return;
    }
    try {
      await axios.post('/api/user/register', form);
      setMsg('注册成功，请登录');
    } catch (err) {
      setMsg(err.response?.data?.error || '注册失败');
    }
  };
  return (
    <div className="form-container">
      <form className="form-card" onSubmit={handleSubmit}>
        <h2>注册</h2>
        <input name="username" placeholder="用户名" value={form.username} onChange={handleChange} />
        <input name="password" type="password" placeholder="密码" value={form.password} onChange={handleChange} />
        <div style={{color: pwdTip === '密码复杂度合规' ? '#27ae60' : '#e67e22', fontSize: '0.95em', minHeight: '1.2em', marginBottom: 4}}>{form.password && pwdTip}</div>
        <div className="form-actions">
          <button type="submit">注册</button>
          <button type="button" className="link-btn" onClick={onSwitch}>去登录</button>
        </div>
        <div className="form-msg">{msg}</div>
      </form>
    </div>
  );
}

function Login({ onLogin, onSwitch }) {
  const [form, setForm] = useState({ username: '', password: '' });
  const [msg, setMsg] = useState('');
  const handleChange = e => setForm(f => ({ ...f, [e.target.name]: e.target.value }));
  const handleSubmit = async e => {
    e.preventDefault();
    setMsg('');
    try {
      const { data } = await axios.post('/api/user/login', form);
      onLogin(data);
    } catch (err) {
      setMsg(err.response?.data?.error || '登录失败');
    }
  };
  return (
    <div className="form-container">
      <form className="form-card" onSubmit={handleSubmit}>
        <h2>登录</h2>
        <input name="username" placeholder="用户名" value={form.username} onChange={handleChange} />
        <input name="password" type="password" placeholder="密码" value={form.password} onChange={handleChange} />
        <div className="form-actions">
          <button type="submit">登录</button>
          <button type="button" className="link-btn" onClick={onSwitch}>去注册</button>
        </div>
        <div className="form-msg">{msg}</div>
      </form>
    </div>
  );
}

function Home({ user, onLogout }) {
  return (
    <div>
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        padding: '16px 24px', background: '#f5f5f5', borderBottom: '1px solid #eee'
      }}>
        <div>
          欢迎，{user.nickname || user.username}（{user.role}）
        </div>
        <button className="logout-btn" onClick={onLogout}>退出登录</button>
      </div>
      <FileListPage />
    </div>
  );
}

export default function App() {
  const [page, setPage] = useState('login');
  const [user, setUser] = useState(() => {
    const t = localStorage.getItem('token');
    const u = localStorage.getItem('user');
    return t && u ? JSON.parse(u) : null;
  });
  const handleLogin = ({ token, user }) => {
    localStorage.setItem('token', token);
    localStorage.setItem('user', JSON.stringify(user));
    setUser(user);
    setPage('home');
  };
  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setUser(null);
    setPage('login');
  };
  if (user) return <Home user={user} onLogout={handleLogout} />;
  return page === 'login' ? (
    <Login onLogin={handleLogin} onSwitch={() => setPage('register')} />
  ) : (
    <Register onSwitch={() => setPage('login')} />
  );
} 