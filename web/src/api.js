import axios from 'axios';

// 获取回收站文件列表
export const getRecycleBinFiles = (page = 1, pageSize = 20) =>
  axios.get('/api/recycle', { params: { page, page_size: pageSize } });

// 还原文件/文件夹
export const restoreFile = (fileId, targetPath = '') =>
  axios.post('/api/recycle/restore', { file_id: fileId, target_path: targetPath });

// 彻底删除文件/文件夹
export const deleteFilePermanently = (fileId) =>
  axios.delete('/api/recycle', { data: { file_id: fileId } });

// 获取临时上传URL
export const getUploadUrl = (filename, size, parentId = '') => 
  axios.get('/api/files/upload-url', { params: { filename, size, parent_id: parentId } });

// 获取临时下载URL
export const getDownloadUrl = (fileId) =>
  axios.get(`/api/files/download-url/${fileId}`);

// 直接上传文件到块存储服务
export const uploadToChunkServer = async (uploadUrl, token, file, onProgress) => {
  const formData = new FormData();
  formData.append('token', token);
  formData.append('file', file);
  
  return axios.post(uploadUrl, formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    },
    onUploadProgress: (progressEvent) => {
      if (onProgress) {
        const percentCompleted = Math.round((progressEvent.loaded * 100) / progressEvent.total);
        onProgress(percentCompleted);
      }
    }
  });
};

// 通知主服务器上传完成
export const notifyUploadComplete = (fileId, hash, size) =>
  axios.post('/api/files/upload-complete', { file_id: fileId, hash, size });

// 直接从块存储服务下载文件
export const downloadFromChunkServer = (downloadUrl, token, filename) => {
  const url = `${downloadUrl}?token=${token}`;
  const link = document.createElement('a');
  link.href = url;
  link.setAttribute('download', filename);
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}; 