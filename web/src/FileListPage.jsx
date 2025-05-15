import React, { useEffect, useState } from 'react';
import { Table, Button, Input, Space, Upload, message, Popconfirm, Breadcrumb } from 'antd';
import { UploadOutlined, DownloadOutlined, DeleteOutlined, FolderOpenOutlined, FileOutlined, HomeOutlined } from '@ant-design/icons';
// import axios from 'axios'; // 暂时不用

const { Search } = Input;

// 支持嵌套的 mock 数据
const mockFilesInit = [
  {
    id: 1,
    name: '文档1.pdf',
    size: 204800,
    uploadTime: '2024-06-01 10:00:00',
    type: 'file',
  },
  {
    id: 2,
    name: '图片2.png',
    size: 102400,
    uploadTime: '2024-06-02 12:30:00',
    type: 'file',
  },
  {
    id: 3,
    name: '项目资料',
    type: 'folder',
    uploadTime: '2024-06-03 09:15:00',
    children: [
      {
        id: 4,
        name: '子文档.docx',
        size: 40960,
        uploadTime: '2024-06-03 10:00:00',
        type: 'file',
      },
      {
        id: 5,
        name: '设计图',
        type: 'folder',
        uploadTime: '2024-06-03 11:00:00',
        children: [
          {
            id: 6,
            name: 'UI.png',
            size: 20480,
            uploadTime: '2024-06-03 11:10:00',
            type: 'file',
          },
        ],
      },
    ],
  },
];

function findFolderByPath(root, pathArr) {
  let current = root;
  for (const id of pathArr) {
    const next = current.find(item => item.id === id && item.type === 'folder');
    if (!next) return [];
    current = next.children || [];
  }
  return current;
}

function findBreadcrumb(root, pathArr) {
  let current = root;
  const crumbs = [];
  for (const id of pathArr) {
    const next = current.find(item => item.id === id && item.type === 'folder');
    if (!next) break;
    crumbs.push(next);
    current = next.children || [];
  }
  return crumbs;
}

const FileListPage = () => {
  const [mockFiles, setMockFiles] = useState(mockFilesInit);
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
    setTimeout(() => {
      let list = findFolderByPath(mockFiles, currentPath);
      if (search) {
        list = list.filter(f => f.name.includes(search));
      }
      setFiles(list.slice((page - 1) * pageSize, page * pageSize));
      setTotal(list.length);
      setLoading(false);
    }, 300);
  };

  useEffect(() => {
    fetchFiles();
    // eslint-disable-next-line
  }, [search, page, pageSize, mockFiles, currentPath]);

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
    message.info(`模拟下载：${file.name}`);
  };

  const handleDelete = async (file) => {
    // 递归删除指定id
    function deleteById(list, id) {
      return list.filter(item => {
        if (item.id === id) return false;
        if (item.type === 'folder' && item.children) {
          item.children = deleteById(item.children, id);
        }
        return true;
      });
    }
    setMockFiles(prev => deleteById([...prev], file.id));
    message.success('模拟删除成功');
  };

  const handleUpload = async ({ file }) => {
    setUploading(true);
    setTimeout(() => {
      // 上传到当前目录
      const newFile = {
        id: Date.now(),
        name: file.name,
        size: file.size || 123456,
        uploadTime: new Date().toLocaleString(),
        type: 'file',
      };
      function addFileToPath(list, pathArr) {
        if (pathArr.length === 0) {
          return [newFile, ...list];
        }
        return list.map(item => {
          if (item.id === pathArr[0] && item.type === 'folder') {
            return {
              ...item,
              children: addFileToPath(item.children || [], pathArr.slice(1)),
            };
          }
          return item;
        });
      }
      setMockFiles(prev => addFileToPath([...prev], currentPath));
      message.success('模拟上传成功');
      setUploading(false);
    }, 500);
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
    ...findBreadcrumb(mockFiles, currentPath).map((folder, idx) => (
      <Breadcrumb.Item key={folder.id} onClick={() => handleBreadcrumbClick(idx + 1)} style={{ cursor: 'pointer' }}>
        <FolderOpenOutlined /> {folder.name}
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