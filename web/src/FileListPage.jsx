import React, { useEffect, useState } from 'react';
import { Table, Button, Input, Space, Upload, message, Popconfirm, Breadcrumb, Modal, Select, Dropdown, Menu, Radio, DatePicker } from 'antd';
import { UploadOutlined, DownloadOutlined, DeleteOutlined, FolderOpenOutlined, FileOutlined, HomeOutlined, MoreOutlined, CopyOutlined, LinkOutlined } from '@ant-design/icons';
import axios from 'axios';
import dayjs from 'dayjs';

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
  const [currentPathNames, setCurrentPathNames] = useState([]); // 路径为名称数组
  const [renameModal, setRenameModal] = useState({ visible: false, file: null, newName: '' });
  const [createFolderModal, setCreateFolderModal] = useState({ visible: false, name: '' });
  const [moveModal, setMoveModal] = useState({ visible: false, file: null, target: '' });
  const [folderOptions, setFolderOptions] = useState([]);
  const [dragOverFolderId, setDragOverFolderId] = useState(null); // 拖拽高亮目标文件夹
  const [shareModal, setShareModal] = useState({ visible: false, file: null, expire: 24, link: '', type: 'public', accessCode: '' });

  // 获取当前目录下的文件和文件夹
  const fetchFiles = async () => {
    setLoading(true);
    try {
      const token = localStorage.getItem('token');
      let url = search ? '/api/files/search' : '/api/files';
      const params = search
        ? { name: search, page, page_size: pageSize }
        : { parent_id: currentPath.length > 0 ? currentPath[currentPath.length - 1] : "", page, page_size: pageSize, order_by: 'upload_time', order: 'desc' };
      const res = await axios.get(url, {
        headers: { Authorization: 'Bearer ' + token },
        params
      });
      setFiles(res.data.files.map(f => ({
        ...f,
        uploadTime: f.upload_time,
      })));
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
    setCurrentPathNames([...currentPathNames, folder.name]);
    setPage(1);
  };

  // 返回上级
  const handleBreadcrumbClick = (idx) => {
    setCurrentPath(currentPath.slice(0, idx));
    setCurrentPathNames(currentPathNames.slice(0, idx));
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
      formData.append('parent_id', currentPath.length > 0 ? currentPath[currentPath.length - 1] : "");
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

  const handleRename = (file) => {
    setRenameModal({ visible: true, file, newName: file.name });
  };

  const doRename = async () => {
    const { file, newName } = renameModal;
    if (!newName || newName === file.name) {
      message.warning('请输入新的文件名');
      return;
    }
    const token = localStorage.getItem('token');
    try {
      await axios.put(`/api/files/${file.id}/rename`, { new_name: newName }, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('重命名成功');
      setRenameModal({ visible: false, file: null, newName: '' });
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '重命名失败');
    }
  };

  const handleCreateFolder = () => {
    setCreateFolderModal({ visible: true, name: '' });
  };

  const doCreateFolder = async () => {
    const { name } = createFolderModal;
    if (!name) {
      message.warning('请输入文件夹名');
      return;
    }
    const token = localStorage.getItem('token');
    try {
      await axios.post('/api/folders', {
        name,
        parent_id: currentPath.length > 0 ? currentPath[currentPath.length - 1] : "",
      }, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('文件夹创建成功');
      setCreateFolderModal({ visible: false, name: '' });
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '文件夹创建失败');
    }
  };

  // 获取所有可选目标文件夹（简单递归/平铺，实际可优化为树）
  const fetchAllFolders = async () => {
    const token = localStorage.getItem('token');
    let all = [{ label: '/', value: '' }]; // 先加根目录
    async function fetchFolderChildren(parentId, path = []) {
      const res = await axios.get('/api/files', {
        headers: { Authorization: 'Bearer ' + token },
        params: { parent_id: parentId, page: 1, page_size: 100 }
      });
      for (const f of res.data.files) {
        if (f.type === 'folder') {
          all.push({ label: [...path, f.name].join('/'), value: f.id });
          await fetchFolderChildren(f.id, [...path, f.name]);
        }
      }
    }
    await fetchFolderChildren('');
    setFolderOptions(all);
  };

  const handleMove = (file) => {
    setMoveModal({ visible: true, file, target: '' }); // target: '' 表示根目录
    fetchAllFolders();
  };

  const doMove = async () => {
    const { file, target } = moveModal;
    if (target === undefined || target === null) { // 允许''作为根目录
      message.warning('请选择目标文件夹');
      return;
    }
    const token = localStorage.getItem('token');
    try {
      await axios.put(`/api/files/${file.id}/move`, { new_parent_id: target }, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('移动成功');
      setMoveModal({ visible: false, file: null, target: '' });
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '移动失败');
    }
  };

  // 拖拽移动文件到文件夹
  const handleMoveByDrag = async (fileId, targetFolderId) => {
    if (!fileId || fileId === targetFolderId) return;
    const token = localStorage.getItem('token');
    try {
      await axios.put(`/api/files/${fileId}/move`, { new_parent_id: targetFolderId }, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('移动成功');
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '移动失败');
    }
  };

  // 进入上一级目录
  const goUpOneLevel = () => {
    if (currentPath.length > 0) {
      setCurrentPath(currentPath.slice(0, -1));
      setCurrentPathNames(currentPathNames.slice(0, -1));
      setPage(1);
    }
  };

  // 拖拽到上一级目录
  const handleMoveToParentByDrag = async (fileId) => {
    if (!fileId || currentPath.length === 0) return;
    const parentId = currentPath.length > 1 ? currentPath[currentPath.length - 2] : '';
    const token = localStorage.getItem('token');
    try {
      await axios.put(`/api/files/${fileId}/move`, { new_parent_id: parentId }, {
        headers: { Authorization: 'Bearer ' + token },
      });
      message.success('移动成功');
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '移动失败');
    }
  };

  // 构造带有"../"的文件列表
  const getDisplayFiles = () => {
    if (currentPath.length === 0) return files;
    // 构造"../"虚拟项
    return [
      {
        id: '__up__',
        name: '../',
        type: 'up',
      },
      ...files,
    ];
  };

  const handleShare = async (file) => {
    let frontendOrigin = window.location.origin;
    if (frontendOrigin.includes(':8080')) {
      frontendOrigin = frontendOrigin.replace(':8080', ':5173');
    }
    // 先查私有
    try {
      const res = await axios.get('/api/share/private', { params: { resource_id: file.id } });
      const token = res.data.share_link.split('/').pop();
      setShareModal({
        visible: true,
        file,
        expire: 24,
        link: `${frontendOrigin}/share/${token}`,
        type: 'private',
        accessCode: res.data.access_code || '',
      });
      message.info('已存在私有分享，直接展示');
      return;
    } catch {}
    // 再查公开
    try {
      const res = await axios.get('/api/share/public', { params: { resource_id: file.id } });
      const token = res.data.share_link.split('/').pop();
      setShareModal({
        visible: true,
        file,
        expire: 24,
        link: `${frontendOrigin}/share/${token}`,
        type: 'public',
        accessCode: '',
      });
      message.info('已存在公开分享，直接展示');
      return;
    } catch {}
    // 否则新建
    setShareModal({ visible: true, file, expire: 24, link: '', type: 'public', accessCode: '' });
  };

  const doShare = async () => {
    const { file, expire, type } = shareModal;
    if (!expire || expire < 1 || expire > 168) {
      message.warning('有效期需为1~168小时');
      return;
    }
    try {
      let res;
      if (type === 'public') {
        res = await axios.post('/api/share/public', {
          resource_id: file.id,
          expire_hours: expire,
        });
      } else {
        res = await axios.post('/api/share/private', {
          resource_id: file.id,
          expire_hours: expire,
        });
      }
      let frontendOrigin = window.location.origin;
      if (frontendOrigin.includes(':8080')) {
        frontendOrigin = frontendOrigin.replace(':8080', ':5173');
      }
      const token = res.data.share_link.split('/').pop();
      setShareModal(s => ({
        ...s,
        link: `${frontendOrigin}/share/${token}`,
        accessCode: res.data.access_code || '',
      }));
      message.success('分享链接已生成');
    } catch (e) {
      message.error(e.response?.data?.error || '分享失败');
    }
  };

  const getShareCopyText = () => {
    if (!shareModal.link) return '';
    if (shareModal.accessCode) {
      return `分享链接：${shareModal.link}\n访问码：${shareModal.accessCode}`;
    }
    return `分享链接：${shareModal.link}`;
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => {
        if (record.type === 'up') {
          // "../"项
          return (
            <span
              style={{
                cursor: 'pointer',
                color: '#faad14',
                fontStyle: 'italic',
                background: dragOverFolderId === '__up__' ? '#fffbe6' : undefined,
                borderRadius: dragOverFolderId === '__up__' ? 4 : undefined,
                padding: dragOverFolderId === '__up__' ? '0 4px' : undefined,
              }}
              onClick={goUpOneLevel}
              onDragOver={e => {
                e.preventDefault();
                setDragOverFolderId('__up__');
              }}
              onDragLeave={e => {
                setDragOverFolderId(null);
              }}
              onDrop={async e => {
                e.preventDefault();
                setDragOverFolderId(null);
                const fileId = e.dataTransfer.getData('fileId');
                if (fileId) {
                  await handleMoveToParentByDrag(fileId);
                }
              }}
            >
              <FolderOpenOutlined style={{ marginRight: 6 }} />../
            </span>
          );
        }
        if (record.type === 'folder') {
          return (
            <span
              style={{
                cursor: 'pointer',
                color: '#1677ff',
                background: dragOverFolderId === record.id ? '#e6f7ff' : undefined,
                borderRadius: dragOverFolderId === record.id ? 4 : undefined,
                padding: dragOverFolderId === record.id ? '0 4px' : undefined,
              }}
              onClick={() => enterFolder(record)}
              onDragOver={e => {
                e.preventDefault();
                setDragOverFolderId(record.id);
              }}
              onDragLeave={e => {
                setDragOverFolderId(null);
              }}
              onDrop={async e => {
                e.preventDefault();
                setDragOverFolderId(null);
                const fileId = e.dataTransfer.getData('fileId');
                if (fileId && fileId !== record.id) {
                  await handleMoveByDrag(fileId, record.id);
                }
              }}
              draggable
              onDragStart={e => {
                e.dataTransfer.setData('fileId', record.id);
                e.dataTransfer.setData('fileName', record.name);
                e.dataTransfer.setData('isFolder', 'true');
              }}
            >
              <FolderOpenOutlined style={{ marginRight: 6 }} />{text}
            </span>
          );
        } else {
          return (
            <span
              draggable
              onDragStart={e => {
                e.dataTransfer.setData('fileId', record.id);
                e.dataTransfer.setData('fileName', record.name);
              }}
            >
              <FileOutlined style={{ marginRight: 6 }} />{text}
            </span>
          );
        }
      },
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
      render: (_, file) => {
        if (file.type === 'up') return null;
        const moreMenu = (
          <Menu>
            {file.type === 'file' && (
              <Menu.Item key="download" onClick={() => handleDownload(file)} icon={<DownloadOutlined />}>
                下载
              </Menu.Item>
            )}
            <Menu.Item key="share" onClick={() => handleShare(file)} icon={<LinkOutlined />}>
              分享
            </Menu.Item>
            <Menu.Item key="rename" onClick={() => handleRename(file)}>
              重命名
            </Menu.Item>
            <Menu.Item key="move" onClick={() => handleMove(file)}>
              移动
            </Menu.Item>
            <Menu.Item key="delete">
              <Popconfirm title="确定删除此项吗？" onConfirm={() => handleDelete(file)}>
                <span style={{ color: '#ff4d4f' }}>删除</span>
              </Popconfirm>
            </Menu.Item>
          </Menu>
        );
        return (
          <Dropdown overlay={moreMenu} trigger={['click']}>
            <Button icon={<MoreOutlined />} />
          </Dropdown>
        );
      },
    },
  ];

  // 面包屑导航
  const breadcrumbs = [
    <Breadcrumb.Item key="root" onClick={() => handleBreadcrumbClick(0)} style={{ cursor: 'pointer' }}>
      <HomeOutlined /> 根目录
    </Breadcrumb.Item>,
    ...currentPath.map((id, idx) => (
      <Breadcrumb.Item key={id} onClick={() => handleBreadcrumbClick(idx + 1)} style={{ cursor: 'pointer' }}>
        <FolderOpenOutlined /> {currentPathNames[idx] || id}
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
          style={{ width: 200 }}
        />
        <Button onClick={handleCreateFolder} type="default">
          新建文件夹
        </Button>
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
        dataSource={getDisplayFiles()}
        rowKey={record => record.id}
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
      <Modal
        title="重命名"
        open={renameModal.visible}
        onOk={doRename}
        onCancel={() => setRenameModal({ visible: false, file: null, newName: '' })}
        okText="确定"
        cancelText="取消"
      >
        <Input
          value={renameModal.newName}
          onChange={e => setRenameModal(r => ({ ...r, newName: e.target.value }))}
          placeholder="请输入新文件名"
          onPressEnter={doRename}
        />
      </Modal>
      <Modal
        title="新建文件夹"
        open={createFolderModal.visible}
        onOk={doCreateFolder}
        onCancel={() => setCreateFolderModal({ visible: false, name: '' })}
        okText="确定"
        cancelText="取消"
      >
        <Input
          value={createFolderModal.name}
          onChange={e => setCreateFolderModal(r => ({ ...r, name: e.target.value }))}
          placeholder="请输入文件夹名"
          onPressEnter={doCreateFolder}
        />
      </Modal>
      <Modal
        title="移动到..."
        open={moveModal.visible}
        onOk={doMove}
        onCancel={() => setMoveModal({ visible: false, file: null, target: '' })}
        okText="确定"
        cancelText="取消"
      >
        <Select
          style={{ width: '100%' }}
          placeholder="请选择目标文件夹"
          value={moveModal.target}
          onChange={v => setMoveModal(m => ({ ...m, target: v }))}
          options={folderOptions.filter(opt => opt.value !== moveModal.file?.id)}
          showSearch
          optionFilterProp="label"
        />
      </Modal>
      <Modal
        title="生成分享链接"
        open={shareModal.visible}
        onOk={doShare}
        onCancel={() => setShareModal({ visible: false, file: null, expire: 24, link: '', type: 'public', accessCode: '' })}
        okText={shareModal.link ? '关闭' : '生成'}
        cancelText="取消"
        footer={shareModal.link ? [
          <Button key="copy" icon={<CopyOutlined />} onClick={() => {navigator.clipboard.writeText(getShareCopyText()); message.success('已复制');}}>复制链接</Button>,
          <Button key="close" type="primary" onClick={() => setShareModal({ visible: false, file: null, expire: 24, link: '', type: 'public', accessCode: '' })}>关闭</Button>
        ] : undefined}
      >
        <div>
          <div style={{ marginBottom: 12 }}>
            <Radio.Group
              value={shareModal.type}
              onChange={e => setShareModal(s => ({ ...s, type: e.target.value }))}
              disabled={!!shareModal.link}
              style={{ marginBottom: 8 }}
            >
              <Radio value="public">公开分享</Radio>
              <Radio value="private">私有分享（需访问码）</Radio>
            </Radio.Group>
          </div>
          <div style={{ marginBottom: 12 }}>
            有效期（小时，1~168）：
            <Input
              type="number"
              min={1}
              max={168}
              value={shareModal.expire}
              onChange={e => setShareModal(s => ({ ...s, expire: Number(e.target.value) }))}
              style={{ width: 100, marginLeft: 8 }}
              disabled={!!shareModal.link}
            />
          </div>
          {shareModal.link && (
            <div style={{ wordBreak: 'break-all', background: '#f6ffed', padding: 8, borderRadius: 4, marginBottom: 8 }}>
              <LinkOutlined /> 分享链接：<a href={shareModal.link} target="_blank" rel="noopener noreferrer">{shareModal.link}</a>
              {shareModal.accessCode && (
                <div style={{ marginTop: 8 }}><b>访问码：</b><span style={{ fontSize: 18, letterSpacing: 2 }}>{shareModal.accessCode}</span></div>
              )}
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
};

export default FileListPage; 