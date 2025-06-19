import React from 'react';
import { Button, message } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import { getDownloadUrl, downloadFromChunkServer } from '../api';

const DirectDownloader = ({ fileId, filename }) => {
  const handleDownload = async () => {
    try {
      // 从主服务器获取临时下载URL和令牌
      const response = await getDownloadUrl(fileId);
      const { download_url, token, filename: serverFilename } = response.data;
      
      // 直接从块存储服务下载文件
      downloadFromChunkServer(download_url, token, serverFilename || filename);
    } catch (error) {
      console.error('下载失败:', error);
      message.error('下载失败: ' + (error.response?.data?.error || error.message));
    }
  };

  return (
    <Button 
      icon={<DownloadOutlined />} 
      onClick={handleDownload}
      type="primary"
    >
      下载
    </Button>
  );
};

export default DirectDownloader; 