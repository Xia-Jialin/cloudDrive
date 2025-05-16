import React, { useEffect, useState } from 'react';
import { Button, Input, message, Spin, Result } from 'antd';
import { LinkOutlined, DownloadOutlined } from '@ant-design/icons';
import axios from 'axios';

export default function ShareDownloadPage() {
  const [loading, setLoading] = useState(true);
  const [file, setFile] = useState(null);
  const [error, setError] = useState('');
  const [token, setToken] = useState('');

  useEffect(() => {
    // 获取token参数
    const urlParams = new URLSearchParams(window.location.search);
    const t = urlParams.get('token') || window.location.pathname.split('/').pop();
    setToken(t);
    if (!t) {
      setError('无效的分享链接');
      setLoading(false);
      return;
    }
    axios.get(`/api/share/${t}`)
      .then(res => {
        setFile(res.data);
        setLoading(false);
      })
      .catch(e => {
        setError(e.response?.data?.error || '无法获取分享信息');
        setLoading(false);
      });
  }, []);

  const handleDownload = () => {
    if (!file) return;
    window.open(`/api/share/download/${token}`);
  };

  if (loading) return <div style={{textAlign:'center',marginTop:80}}><Spin size="large" /></div>;
  if (error) return <Result status="error" title="无法访问分享" subTitle={error} />;
  return (
    <div style={{maxWidth:420,margin:'60px auto',padding:32,boxShadow:'0 2px 16px #eee',borderRadius:12,background:'#fff'}}>
      <h2><LinkOutlined /> 文件分享</h2>
      <div style={{margin:'16px 0',fontSize:18}}><b>文件名：</b>{file.name}</div>
      <div style={{margin:'8px 0'}}><b>类型：</b>{file.type==='file'?'文件':'文件夹'}</div>
      <div style={{margin:'8px 0'}}><b>分享有效期至：</b>{new Date(file.expire_at*1000).toLocaleString()}</div>
      <Button type="primary" icon={<DownloadOutlined />} size="large" onClick={handleDownload}>
        下载文件
      </Button>
    </div>
  );
} 