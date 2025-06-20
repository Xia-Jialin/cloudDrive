package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v4"
)

// ChunkServerStorage 存储服务客户端
type ChunkServerStorage struct {
	BaseURL     string        // 存储服务基础URL
	PublicURL   string        // 存储服务公共URL（返回给前端）
	RedisClient *redis.Client // Redis客户端
	TempDir     string        // 临时目录
	SecretKey   string        // JWT密钥
	HTTPClient  *http.Client  // HTTP客户端
}

// NewChunkServerStorage 创建存储服务客户端
func NewChunkServerStorage(baseURL string, redisClient *redis.Client, tempDir string) (*ChunkServerStorage, error) {
	// 从配置中获取JWT密钥，这里简化处理，实际应该从配置文件或环境变量获取
	secretKey := "your-super-secret-key-for-jwt-token-signing"

	return &ChunkServerStorage{
		BaseURL:     baseURL,
		PublicURL:   baseURL, // 默认与BaseURL相同
		RedisClient: redisClient,
		TempDir:     tempDir,
		SecretKey:   secretKey,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SetPublicURL 设置公共URL
func (c *ChunkServerStorage) SetPublicURL(publicURL string) {
	c.PublicURL = publicURL
}

// Upload 实现Storage接口的Upload方法
func (c *ChunkServerStorage) Upload(ctx context.Context, key string, content io.Reader) error {
	// 创建临时文件
	tempFile := filepath.Join(c.TempDir, key)
	if err := os.MkdirAll(filepath.Dir(tempFile), 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer file.Close()

	// 将内容写入临时文件
	if _, err := io.Copy(file, content); err != nil {
		return fmt.Errorf("写入临时文件失败: %v", err)
	}

	// 生成临时上传令牌
	token, err := c.GenerateUploadToken(map[string]interface{}{
		"file_id": key,
	}, 3600)
	if err != nil {
		return fmt.Errorf("生成上传令牌失败: %v", err)
	}

	// 上传文件
	_, err = c.UploadFile(tempFile, token)
	if err != nil {
		return fmt.Errorf("上传文件失败: %v", err)
	}

	// 删除临时文件
	os.Remove(tempFile)

	return nil
}

// Download 实现Storage接口的Download方法
func (c *ChunkServerStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// 生成临时下载令牌
	token, err := c.GenerateDownloadToken(key, filepath.Base(key), 3600)
	if err != nil {
		return nil, fmt.Errorf("生成下载令牌失败: %v", err)
	}

	// 构建下载URL
	downloadURL := fmt.Sprintf("%s/download?token=%s", c.BaseURL, url.QueryEscape(token))

	// 发送HTTP请求
	resp, err := c.HTTPClient.Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("发送下载请求失败: %v", err)
	}

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// Delete 实现Storage接口的Delete方法
func (c *ChunkServerStorage) Delete(ctx context.Context, key string) error {
	// 构建删除URL
	deleteURL := fmt.Sprintf("%s/api/file/%s", c.BaseURL, url.PathEscape(key))

	// 创建DELETE请求
	req, err := http.NewRequestWithContext(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("创建删除请求失败: %v", err)
	}

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送删除请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("删除失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// InitMultipartUpload 实现Storage接口的InitMultipartUpload方法
func (c *ChunkServerStorage) InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error) {
	// 构建初始化URL
	initURL := fmt.Sprintf("%s/api/multipart/init", c.PublicURL)

	// 创建请求体
	reqBody := map[string]string{
		"file_id":  fileID,
		"filename": filename,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 创建POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", initURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建初始化请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送初始化请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("初始化分片上传失败，状态码: %d，响应: %s", resp.StatusCode, string(bodyBytes))
	}

	// 读取整个响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %v", err)
	}

	// 打印响应内容，用于调试
	log.Printf("块存储服务响应: %s", string(bodyBytes))

	// 尝试解析块存储服务的响应格式
	var chunkServerResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			UploadID  string `json:"upload_id"`
			ServerURL string `json:"server_url,omitempty"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &chunkServerResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(bodyBytes))
	}

	// 检查响应码
	if chunkServerResp.Code != 0 {
		return "", fmt.Errorf("初始化分片上传失败: %s", chunkServerResp.Message)
	}

	// 提取上传ID
	uploadID := chunkServerResp.Data.UploadID
	if uploadID == "" {
		return "", fmt.Errorf("响应中没有上传ID: %s", string(bodyBytes))
	}

	// 如果服务器返回了URL，使用它替换内部URL
	if chunkServerResp.Data.ServerURL != "" {
		c.PublicURL = chunkServerResp.Data.ServerURL
	}

	return uploadID, nil
}

// UploadPart 实现Storage接口的UploadPart方法
func (c *ChunkServerStorage) UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader, options ...interface{}) (string, error) {
	// 创建临时文件
	tempFile := filepath.Join(c.TempDir, fmt.Sprintf("%s-%d", uploadID, partNumber))
	file, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer func() {
		file.Close()
		os.Remove(tempFile)
	}()

	// 将分片数据写入临时文件
	if _, err := io.Copy(file, partData); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %v", err)
	}
	file.Close()

	// 重新打开文件以读取
	file, err = os.Open(tempFile)
	if err != nil {
		return "", fmt.Errorf("打开临时文件失败: %v", err)
	}
	defer file.Close()

	// 构建上传URL
	uploadURL := fmt.Sprintf("%s/api/multipart/part", c.PublicURL)

	// 创建multipart表单
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加uploadID字段
	if err := writer.WriteField("upload_id", uploadID); err != nil {
		return "", fmt.Errorf("添加uploadID字段失败: %v", err)
	}

	// 添加partNumber字段
	if err := writer.WriteField("part_number", strconv.Itoa(partNumber)); err != nil {
		return "", fmt.Errorf("添加partNumber字段失败: %v", err)
	}

	// 如果提供了token，添加token字段
	if len(options) > 0 {
		if token, ok := options[0].(string); ok && token != "" {
			if err := writer.WriteField("token", token); err != nil {
				return "", fmt.Errorf("添加token字段失败: %v", err)
			}
		}
	}

	// 添加文件字段
	part, err := writer.CreateFormFile("part", fmt.Sprintf("part-%d", partNumber))
	if err != nil {
		return "", fmt.Errorf("创建文件字段失败: %v", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("复制文件内容失败: %v", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("关闭writer失败: %v", err)
	}

	// 创建POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, body)
	if err != nil {
		return "", fmt.Errorf("创建上传请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送上传请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("上传分片失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ETag string `json:"etag"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	// 检查响应码
	if result.Code != 0 {
		return "", fmt.Errorf("上传分片失败: %s", result.Message)
	}

	return result.Data.ETag, nil
}

// CompleteMultipartUpload 实现Storage接口的CompleteMultipartUpload方法
func (c *ChunkServerStorage) CompleteMultipartUpload(ctx context.Context, uploadID string, parts []PartInfo) (string, error) {
	// 构建完成URL
	completeURL := fmt.Sprintf("%s/api/multipart/complete", c.PublicURL)

	// 创建请求体
	reqBody := struct {
		UploadID string     `json:"upload_id"`
		Parts    []PartInfo `json:"parts"`
	}{
		UploadID: uploadID,
		Parts:    parts,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 创建POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", completeURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建完成请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送完成请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("完成分片上传失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		FileHash string `json:"file_hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	return result.FileHash, nil
}

// ListUploadedParts 实现Storage接口的ListUploadedParts方法
func (c *ChunkServerStorage) ListUploadedParts(ctx context.Context, uploadID string) ([]int, error) {
	// 构建请求URL
	listURL := fmt.Sprintf("%s/api/multipart/status?upload_id=%s", c.PublicURL, url.QueryEscape(uploadID))

	// 创建GET请求
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取分片列表失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Parts []int `json:"parts"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	// 检查响应码
	if result.Code != 0 {
		return nil, fmt.Errorf("获取分片列表失败: %s", result.Message)
	}

	return result.Data.Parts, nil
}

// GenerateUploadToken 生成上传令牌
func (c *ChunkServerStorage) GenerateUploadToken(uploadInfo map[string]interface{}, expireSeconds int) (string, error) {
	// 创建令牌
	claims := jwt.MapClaims{
		"file_id":   uploadInfo["file_id"],
		"user_id":   uploadInfo["user_id"],
		"filename":  uploadInfo["filename"],
		"size":      uploadInfo["size"],
		"parent_id": uploadInfo["parent_id"],
		"exp":       time.Now().Add(time.Duration(expireSeconds) * time.Second).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(c.SecretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GenerateDownloadToken 生成下载令牌
func (c *ChunkServerStorage) GenerateDownloadToken(fileHash string, filename string, expireSeconds int) (string, error) {
	// 创建令牌
	claims := jwt.MapClaims{
		"file_hash": fileHash,
		"filename":  filename,
		"exp":       time.Now().Add(time.Duration(expireSeconds) * time.Second).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(c.SecretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetBaseURL 获取块存储服务的基础URL
func (c *ChunkServerStorage) GetBaseURL() string {
	return c.BaseURL
}

// GetPublicURL 返回公共URL
func (c *ChunkServerStorage) GetPublicURL() string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	return c.BaseURL
}

// CalculateFileHash 计算文件的SHA256哈希值
func (c *ChunkServerStorage) CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 使用SHA256算法
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算哈希失败: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// UploadFile 通过块存储服务上传文件
func (c *ChunkServerStorage) UploadFile(filePath string, token string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加token字段
	if err := writer.WriteField("token", token); err != nil {
		return "", err
	}

	// 添加文件字段
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/upload", c.BaseURL), body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Hash string `json:"hash"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Hash, nil
}

// DownloadFile 通过块存储服务下载文件
func (c *ChunkServerStorage) DownloadFile(fileID string, token string, destPath string) error {
	params := url.Values{}
	params.Add("token", token)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/download?%s", c.BaseURL, params.Encode()), nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("下载失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
