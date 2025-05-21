import React, { useState } from 'react';
import { Modal, Tree, Input, Alert } from 'antd';
import axios from 'axios';

// 递归更新树节点
const updateTreeData = (list, key, children) =>
  list.map(node => {
    if (node.key === key) {
      return { ...node, children };
    }
    if (node.children) {
      return { ...node, children: updateTreeData(node.children, key, children) };
    }
    return node;
  });

// 只显示文件夹节点的目录树弹窗
const TreeSelectModal = ({ visible, onSelect, onCancel, error }) => {
  const [treeData, setTreeData] = useState([
    { title: '/', key: '', isLeaf: false, children: [] }
  ]);
  const [selectedKey, setSelectedKey] = useState('');
  const [loadingKeys, setLoadingKeys] = useState([]);
  const [search, setSearch] = useState('');

  // 懒加载子目录
  const onLoadData = async (treeNode) => {
    const { key } = treeNode;
    if (treeNode.children && treeNode.children.length > 0) return;
    setLoadingKeys(keys => [...keys, key]);
    try {
      const res = await axios.get('/api/files', {
        params: { parent_id: key, page: 1, page_size: 100 },
        withCredentials: true
      });
      const folders = res.data.files.filter(f => f.type === 'folder');
      const children = folders.map(f => ({
        title: f.name,
        key: f.id,
        isLeaf: false,
        children: []
      }));
      setTreeData(origin => updateTreeData(origin, key, children));
    } finally {
      setLoadingKeys(keys => keys.filter(k => k !== key));
    }
  };

  // 选择节点
  const onSelectNode = (keys) => {
    setSelectedKey(keys[0]);
  };

  // 确定
  const handleOk = () => {
    onSelect(selectedKey);
  };

  // 取消
  const handleCancel = () => {
    setSelectedKey('');
    onCancel();
  };

  return (
    <Modal
      title="选择还原路径"
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ disabled: !selectedKey && selectedKey !== '' }}
      cancelText="取消"
      okText="确定"
    >
      {error && <Alert type="error" message={error} style={{ marginBottom: 12 }} />}
      <Input.Search
        placeholder="搜索目录"
        value={search}
        onChange={e => setSearch(e.target.value)}
        style={{ marginBottom: 8 }}
        allowClear
      />
      <Tree
        treeData={treeData}
        loadData={onLoadData}
        onSelect={onSelectNode}
        selectedKeys={[selectedKey]}
        showLine
        defaultExpandAll
        height={320}
      />
    </Modal>
  );
};

export default TreeSelectModal; 