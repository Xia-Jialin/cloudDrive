import React, { useEffect, useState } from 'react';
import { Modal, Spin, message } from 'antd';

export default function FilePreviewModal({ file, visible, onClose }) {
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(false);
  const [blobUrl, setBlobUrl] = useState('');

  useEffect(() => {
    if (!file || !visible) return;
    setBlobUrl('');
    if (file.type.startsWith('image/')) {
      setLoading(true);
      fetch(`/api/files/preview/${file.id}`, {
        credentials: 'include'
      })
        .then(res => res.blob())
        .then(blob => setBlobUrl(URL.createObjectURL(blob)))
        .catch(() => message.error('无法加载图片'))
        .finally(() => setLoading(false));
    } else if (file.type === 'application/pdf') {
      setLoading(true);
      fetch(`/api/files/preview/${file.id}`, {
        credentials: 'include'
      })
        .then(res => res.blob())
        .then(blob => setBlobUrl(URL.createObjectURL(blob)))
        .catch(() => message.error('无法加载PDF'))
        .finally(() => setLoading(false));
    } else if (file.type.startsWith('video/') || file.type.startsWith('audio/')) {
      setLoading(true);
      fetch(`/api/files/preview/${file.id}`, {
        credentials: 'include'
      })
        .then(res => res.blob())
        .then(blob => setBlobUrl(URL.createObjectURL(blob)))
        .catch(() => message.error('无法加载媒体文件'))
        .finally(() => setLoading(false));
    } else if (file.type.startsWith('text/')) {
      setLoading(true);
      fetch(`/api/files/preview/${file.id}`, {
        credentials: 'include'
      })
        .then(res => res.text())
        .then(setContent)
        .catch(() => message.error('无法加载文本内容'))
        .finally(() => setLoading(false));
    }
    return () => {
      if (blobUrl) URL.revokeObjectURL(blobUrl);
    };
    // eslint-disable-next-line
  }, [file, visible]);

  if (!file) return null;

  let body = null;
  if (file.type.startsWith('image/')) {
    body = loading ? <Spin /> : <img src={blobUrl} alt={file.name} style={{ maxWidth: '100%', maxHeight: 500 }} />;
  } else if (file.type === 'application/pdf') {
    body = loading ? <Spin /> : (
      <iframe
        src={blobUrl}
        title={file.name}
        width="100%"
        height="500px"
        style={{ border: 'none' }}
      />
    );
  } else if (file.type.startsWith('video/')) {
    body = loading ? <Spin /> : (
      <video controls style={{ maxWidth: '100%', maxHeight: 500 }}>
        <source src={blobUrl} type={file.type} />
        您的浏览器不支持视频播放
      </video>
    );
  } else if (file.type.startsWith('audio/')) {
    body = loading ? <Spin /> : (
      <audio controls style={{ width: '100%' }}>
        <source src={blobUrl} type={file.type} />
        您的浏览器不支持音频播放
      </audio>
    );
  } else if (file.type.startsWith('text/')) {
    body = loading ? <Spin /> : <pre style={{ maxHeight: 400, overflow: 'auto', background: '#f6f8fa', padding: 12 }}>{content}</pre>;
  } else {
    body = <div>暂不支持预览该类型文件</div>;
  }

  return (
    <Modal open={visible} onCancel={onClose} footer={null} width={700} title={`预览：${file.name}`}>
      {body}
    </Modal>
  );
} 