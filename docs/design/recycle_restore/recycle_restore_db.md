# 回收站还原功能数据库设计文档

## 1. ER图描述

- 用户(User) 1 --- n 文件(File)
- 文件(File) n --- 1 父文件夹(File, self reference)
- 文件(File) 1 --- 1 文件内容(FileContent)

## 2. 文件表结构（file）

| 字段名         | 类型         | 说明               |
| -------------- | ------------ | ------------------ |
| id             | bigint       | 主键，自增         |
| user_id        | bigint       | 所属用户ID         |
| name           | varchar(255) | 文件/文件夹名      |
| is_dir         | bool         | 是否为文件夹       |
| parent_id      | bigint       | 父文件夹ID         |
| size           | bigint       | 文件大小           |
| is_deleted     | bool         | 是否被删除         |
| deleted_at     | datetime     | 删除时间           |
| original_path  | varchar(1024)| 删除前原始路径     |
| created_at     | datetime     | 创建时间           |
| updated_at     | datetime     | 更新时间           |
| ...            | ...          | 其他业务字段       |

## 3. 字段说明
- `is_deleted`：0=正常，1=回收站
- `deleted_at`：进入回收站的时间
- `original_path`：删除前的完整路径，便于还原

## 4. 索引建议
- user_id + is_deleted：加速用户回收站/正常文件查询
- parent_id：加速目录结构遍历
- deleted_at：加速定时清理

## 5. 示例SQL
```sql
CREATE TABLE `file` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `name` varchar(255) NOT NULL,
  `is_dir` bool NOT NULL DEFAULT 0,
  `parent_id` bigint DEFAULT NULL,
  `size` bigint DEFAULT 0,
  `is_deleted` bool NOT NULL DEFAULT 0,
  `deleted_at` datetime DEFAULT NULL,
  `original_path` varchar(1024) DEFAULT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_deleted` (`user_id`, `is_deleted`),
  KEY `idx_parent` (`parent_id`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 6. 其他说明
- 彻底删除时，物理删除记录及存储文件
- 还原时，`is_deleted`置0，`deleted_at`清空，`parent_id`和`original_path`根据还原路径调整
- 还原到新路径时需校验目标路径有效性和权限 