import React, { useState } from 'react';
import { 
  getUploadUrl, 
  uploadToChunkServer, 
  notifyUploadComplete 
} from '../api';
import { Button, Progress, message, Upload, Space } from 'antd';
import { UploadOutlined } from '@ant-design/icons';

const DirectUploader = ({ parentId = '', onUploadSuccess }) => {
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [fileList, setFileList] = useState([]);

  const handleUpload = async () => {
    if (fileList.length === 0) {
      message.warning('请先选择文件');
      return;
    }

    const file = fileList[0];
    setUploading(true);
    setProgress(0);

    try {
      // 1. 从主服务器获取临时上传URL和令牌
      const urlResponse = await getUploadUrl(file.name, file.size, parentId);
      const { upload_url, token, file_id } = urlResponse.data;

      // 2. 直接上传文件到块存储服务
      const uploadResponse = await uploadToChunkServer(upload_url, token, file, setProgress);
      const { hash, size } = uploadResponse.data;

      // 3. 通知主服务器上传完成
      await notifyUploadComplete(file_id, hash, size);

      message.success('上传成功');
      setFileList([]);
      
      // 如果提供了成功回调，则调用
      if (onUploadSuccess) {
        onUploadSuccess();
      }
    } catch (error) {
      console.error('上传失败:', error);
      message.error('上传失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setUploading(false);
    }
  };

  const props = {
    onRemove: () => {
      setFileList([]);
    },
    beforeUpload: (file) => {
      setFileList([file]);
      return false;
    },
    fileList,
    multiple: false,
  };

  return (
    <div style={{ marginBottom: 16 }}>
      <Space direction="vertical" style={{ width: '100%' }}>
        <Upload {...props}>
          <Button icon={<UploadOutlined />} disabled={uploading || fileList.length > 0}>
            选择文件
          </Button>
        </Upload>
        
        {fileList.length > 0 && (
          <Button
            type="primary"
            onClick={handleUpload}
            loading={uploading}
            style={{ marginTop: 16 }}
          >
            {uploading ? '上传中' : '开始上传'}
          </Button>
        )}
        
        {uploading && <Progress percent={progress} />}
      </Space>
    </div>
  );
};

export default DirectUploader; 