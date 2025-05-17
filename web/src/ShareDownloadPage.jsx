import React, { useEffect, useState } from 'react';
import { Button, Input, message, Spin, Result } from 'antd';
import { LinkOutlined, DownloadOutlined } from '@ant-design/icons';
import axios from 'axios';

export default function ShareDownloadPage() {
  const [loading, setLoading] = useState(true);
  const [file, setFile] = useState(null);
  const [error, setError] = useState('');
  const [token, setToken] = useState('');
  const [accessCode, setAccessCode] = useState('');
  const [codeRequired, setCodeRequired] = useState(false);
  const [codeChecked, setCodeChecked] = useState(false);
  const [checking, setChecking] = useState(false);

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
        setCodeRequired(false);
        setCodeChecked(true);
        setLoading(false);
      })
      .catch(e => {
        // 检查是否需要访问码
        if (e.response?.status === 401) {
          setCodeRequired(true);
        } else if (e.response?.status === 403) {
          setError('访问码错误');
        } else {
          setError(e.response?.data?.error || '无法获取分享信息');
        }
        setLoading(false);
      });
  }, []);

  const checkAccessCode = async () => {
    setChecking(true);
    setError('');
    try {
      const res = await axios.get(`/api/share/${token}?access_code=${accessCode}`);
      setFile(res.data);
      setCodeChecked(true);
      setCodeRequired(false);
    } catch (e) {
      setError(e.response?.data?.error || '访问码错误');
    }
    setChecking(false);
  };

  const handleDownload = () => {
    if (!file) return;
    let url = `/api/share/download/${token}`;
    if (file && file.type === 'file' && accessCode) {
      url += `?access_code=${accessCode}`;
    }
    window.open(url);
  };

  if (loading) return <div style={{textAlign:'center',marginTop:80}}><Spin size="large" /></div>;
  if (error) return <Result status="error" title="无法访问分享" subTitle={error} />;
  return (
    <div style={{maxWidth:420,margin:'60px auto',padding:32,boxShadow:'0 2px 16px #eee',borderRadius:12,background:'#fff'}}>
      <h2><LinkOutlined /> 文件分享</h2>
      {codeRequired && !codeChecked ? (
        <div>
          <div style={{margin:'16px 0',fontSize:16}}><b>请输入访问码：</b></div>
          <Input
            style={{width:120,marginRight:8}}
            maxLength={4}
            value={accessCode}
            onChange={e => setAccessCode(e.target.value)}
            placeholder="4位码"
            onPressEnter={checkAccessCode}
            disabled={checking}
          />
          <Button type="primary" onClick={checkAccessCode} loading={checking} disabled={!accessCode || accessCode.length !== 4}>
            校验
          </Button>
        </div>
      ) : file && (
        <>
          <div style={{margin:'16px 0',fontSize:18}}><b>文件名：</b>{file.name}</div>
          <div style={{margin:'8px 0'}}><b>类型：</b>{file.type==='file'?'文件':'文件夹'}</div>
          <div style={{margin:'8px 0'}}><b>分享有效期至：</b>{new Date(file.expire_at*1000).toLocaleString()}</div>
          <Button type="primary" icon={<DownloadOutlined />} size="large" onClick={handleDownload}>
            下载文件
          </Button>
        </>
      )}
    </div>
  );
} 