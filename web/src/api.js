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