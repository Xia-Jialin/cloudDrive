import React, { useEffect, useState } from 'react';
import { Table, Button, Input, Space, Upload, message, Popconfirm, Breadcrumb } from 'antd';
import { UploadOutlined, DownloadOutlined, DeleteOutlined, FolderOpenOutlined, FileOutlined, HomeOutlined } from '@ant-design/icons';
import axios from 'axios';

const { Search } = Input;

const FileListPage = () => {
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [currentPath, setCurrentPath] = useState([]); // 路径为id数组

  // 获取当前目录下的文件和文件夹
  const fetchFiles = async () => {
    setLoading(true);
    try {
      const token = localStorage.getItem('token');
      const res = await axios.get('/api/files', {
        headers: { Authorization: 'Bearer ' + token },
        params: {
          parent_id: currentPath.length > 0 ? currentPath[currentPath.length - 1] : 0,
          page,
          page_size: pageSize,
          order_by: 'upload_time',
          order: 'desc',
          // 可加 search 字段，后端支持时
        }
      });
      setFiles(res.data.files);
      setTotal(res.data.total);
    } catch (e) {
      message.error('获取文件列表失败');
    }
    setLoading(false);
  };

  useEffect(() => {
    fetchFiles();
    // eslint-disable-next-line
  }, [search, page, pageSize, currentPath]);

  // 进入文件夹
  const enterFolder = (folder) => {
    setCurrentPath([...currentPath, folder.id]);
    setPage(1);
  };

  // 返回上级
  const handleBreadcrumbClick = (idx) => {
    setCurrentPath(currentPath.slice(0, idx));
    setPage(1);
  };

  const handleDownload = (file) => {
    const token = localStorage.getItem('token');
    if (!token) {
      message.error('请先登录');
      return;
    }
    // 创建隐藏a标签实现下载
    const url = `/api/files/download/${file.id}`;
    fetch(url, {
      headers: { Authorization: 'Bearer ' + token },
    })
      .then(res => {
        if (!res.ok) throw new Error('下载失败');
        return res.blob();
      })
      .then(blob => {
        const a = document.createElement('a');
        a.href = window.URL.createObjectURL(blob);
        a.download = file.name;
        a.style.display = 'none';
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(a.href);
        document.body.removeChild(a);
      })
      .catch(e => {
        message.error(e.message || '下载失败');
      });
  };

  const handleDelete = async (file) => {
    const token = localStorage.getItem('token');
    if (!token) {
      message.error('请先登录');
      return;
    }
    try {
      await axios.delete(`/api/files/${file.id}`, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('删除成功');
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '删除失败');
    }
  };

  const handleUpload = async ({ file }) => {
    setUploading(true);
    try {
      const token = localStorage.getItem('token');
      const formData = new FormData();
      formData.append('file', file);
      formData.append('parent_id', currentPath.length > 0 ? currentPath[currentPath.length - 1] : 0);
      await axios.post('/api/files/upload', formData, {
        headers: {
          Authorization: 'Bearer ' + token,
          'Content-Type': 'multipart/form-data',
        },
      });
      message.success('上传成功');
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '上传失败');
    }
    setUploading(false);
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text, record) =>
        record.type === 'folder' ? (
          <span style={{ cursor: 'pointer', color: '#1677ff' }} onClick={() => enterFolder(record)}>
            <FolderOpenOutlined style={{ marginRight: 6 }} />{text}
          </span>
        ) : (
          <span><FileOutlined style={{ marginRight: 6 }} />{text}</span>
        ),
    },
    {
      title: '大小',
      dataIndex: 'size',
      key: 'size',
      render: (size, record) => record.type === 'file' ? `${(size / 1024).toFixed(2)} KB` : '--',
    },
    {
      title: '上传时间',
      dataIndex: 'uploadTime',
      key: 'uploadTime',
      render: (t) => t ? t.replace('T', ' ').slice(0, 19) : '',
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type) => type === 'folder' ? '文件夹' : '文件',
    },
    {
      title: '操作',
      key: 'action',
      render: (_, file) => (
        <Space>
          {file.type === 'file' && (
            <Button icon={<DownloadOutlined />} onClick={() => handleDownload(file)}>
              下载
            </Button>
          )}
          <Popconfirm title="确定删除此项吗？" onConfirm={() => handleDelete(file)}>
            <Button icon={<DeleteOutlined />} danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  // 面包屑导航
  const breadcrumbs = [
    <Breadcrumb.Item key="root" onClick={() => handleBreadcrumbClick(0)} style={{ cursor: 'pointer' }}>
      <HomeOutlined /> 根目录
    </Breadcrumb.Item>,
    ...currentPath.map((id, idx) => (
      <Breadcrumb.Item key={id} onClick={() => handleBreadcrumbClick(idx + 1)} style={{ cursor: 'pointer' }}>
        <FolderOpenOutlined /> {id}
      </Breadcrumb.Item>
    )),
  ];

  return (
    <div style={{ padding: 24 }}>
      <Breadcrumb style={{ marginBottom: 16 }}>{breadcrumbs}</Breadcrumb>
      <Space style={{ marginBottom: 16 }}>
        <Search
          placeholder="搜索文件名"
          onSearch={setSearch}
          enterButton
          allowClear
        />
        <Upload
          customRequest={handleUpload}
          showUploadList={false}
          disabled={uploading}
        >
          <Button icon={<UploadOutlined />} loading={uploading} type="primary">
            上传文件
          </Button>
        </Upload>
      </Space>
      <Table
        columns={columns}
        dataSource={files}
        rowKey="id"
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          onChange: (p, ps) => {
            setPage(p);
            setPageSize(ps);
          },
        }}
      />
    </div>
  );
};

export default FileListPage; 