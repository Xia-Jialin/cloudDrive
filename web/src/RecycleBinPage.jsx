import React, { useEffect, useState } from 'react';
import { Table, Button, message, Popconfirm } from 'antd';
import dayjs from 'dayjs';
import { getRecycleBinFiles, restoreFile, deleteFilePermanently } from './api';
import './RecycleBinPage.css';
import TreeSelectModal from './components/TreeSelectModal';

function RecycleBinPage() {
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(false);
  const [treeModal, setTreeModal] = useState({ visible: false, fileId: null, error: '' });

  const fetchFiles = async () => {
    setLoading(true);
    try {
      const res = await getRecycleBinFiles();
      setFiles(res.data);
    } catch (e) {
      message.error(e.response?.data?.error || '获取回收站文件失败');
    }
    setLoading(false);
  };

  useEffect(() => {
    fetchFiles();
  }, []);

  const handleRestore = async (fileId) => {
    try {
      await restoreFile(fileId);
      message.success('还原成功');
      fetchFiles();
    } catch (e) {
      if (e.response?.data?.error?.includes('原路径不存在')) {
        setTreeModal({ visible: true, fileId, error: '' });
      } else {
        message.error(e.response?.data?.error || '还原失败');
      }
    }
  };

  // 目录树弹窗选择后回调
  const handleTreeSelect = async (targetPath) => {
    const { fileId } = treeModal;
    try {
      await restoreFile(fileId, targetPath);
      message.success('还原成功');
      setTreeModal({ visible: false, fileId: null, error: '' });
      fetchFiles();
    } catch (e) {
      setTreeModal(modal => ({ ...modal, error: e.response?.data?.error || '还原失败' }));
    }
  };

  const handleTreeCancel = () => {
    setTreeModal({ visible: false, fileId: null, error: '' });
  };

  const handleDelete = async (fileId) => {
    try {
      await deleteFilePermanently(fileId);
      message.success('彻底删除成功');
      fetchFiles();
    } catch (e) {
      message.error(e.response?.data?.error || '彻底删除失败');
    }
  };

  const columns = [
    {
      title: '文件名',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '删除时间',
      dataIndex: 'deleted_at',
      key: 'deleted_at',
      render: t => dayjs(t).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '原路径',
      dataIndex: 'original_path',
      key: 'original_path',
    },
    {
      title: '操作',
      key: 'action',
      render: (_, file) => (
        <>
          <Button
            type="primary"
            size="small"
            style={{ marginRight: 8 }}
            onClick={() => handleRestore(file.id)}
          >
            还原
          </Button>
          <Popconfirm
            title="确定要彻底删除该文件吗？"
            onConfirm={() => handleDelete(file.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button danger size="small">彻底删除</Button>
          </Popconfirm>
        </>
      ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <h2 style={{ fontWeight: 600, fontSize: 22, marginBottom: 20 }}>回收站</h2>
      <Table
        columns={columns}
        dataSource={files}
        rowKey={record => record.id}
        loading={loading}
        pagination={false}
        bordered
        style={{ background: '#fff', borderRadius: 8 }}
      />
      <TreeSelectModal
        visible={treeModal.visible}
        onSelect={handleTreeSelect}
        onCancel={handleTreeCancel}
        error={treeModal.error}
      />
    </div>
  );
}

export default RecycleBinPage; 