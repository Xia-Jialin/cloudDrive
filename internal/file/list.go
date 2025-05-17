package file

import (
	"gorm.io/gorm"
)

type ListFilesRequest struct {
	ParentID   string // 指定目录
	OwnerID    uint   // 当前用户
	Page       int    // 页码
	PageSize   int    // 每页数量
	OrderBy    string // 排序字段
	Order      string // asc/desc
	Name       string // 文件名
	Type       string // 文件类型
	UploadTime string // 上传时间
}

type ListFilesResponse struct {
	Files []File `json:"files"`
	Total int64  `json:"total"`
}

func ListFiles(db *gorm.DB, req ListFilesRequest) (*ListFilesResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 10
	}

	parentID := req.ParentID
	var query *gorm.DB
	if parentID == "" && req.Name != "" {
		// 全局搜索
		query = db.Model(&File{}).Where("owner_id = ?", req.OwnerID)
	} else {
		if parentID == "" {
			// 查询用户根目录ID
			var userRoot UserRoot
			err := db.First(&userRoot, "user_id = ?", req.OwnerID).Error
			if err != nil {
				return nil, err
			}
			parentID = userRoot.RootID
		}
		query = db.Model(&File{}).Where("parent_id = ?", parentID)
	}
	if req.Name != "" {
		query = query.Where("name LIKE ?", "%"+req.Name+"%")
	}
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}
	if req.UploadTime != "" {
		query = query.Where("DATE(upload_time) = ?", req.UploadTime)
	}
	if req.OrderBy != "" {
		order := req.OrderBy
		if req.Order == "desc" {
			order += " desc"
		}
		query = query.Order(order)
	}
	var files []File
	var total int64
	query.Count(&total)
	err := query.Offset((req.Page - 1) * req.PageSize).Limit(req.PageSize).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return &ListFilesResponse{Files: files, Total: total}, nil
}
