package file

import (
	"gorm.io/gorm"
)

type ListFilesRequest struct {
	ParentID string // 指定目录
	OwnerID  uint   // 当前用户
	Page     int    // 页码
	PageSize int    // 每页数量
	OrderBy  string // 排序字段
	Order    string // asc/desc
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
	query := db.Model(&File{}).Where("parent_id = ? AND owner_id = ?", req.ParentID, req.OwnerID)
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
