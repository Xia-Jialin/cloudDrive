package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ChunkServerClient 存储服务客户端
type ChunkServerClient struct {
	BaseURL    string        // 存储服务基础URL，例如 http://localhost:8081
	HTTPClient *http.Client  // HTTP客户端
	Timeout    time.Duration // 请求超时时间
}

// NewChunkServerClient 创建存储服务客户端
func NewChunkServerClient(baseURL string, timeout time.Duration) *ChunkServerClient {
	return &ChunkServerClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
		Timeout: timeout,
	}
}

// 上传文件
func (c *ChunkServerClient) Upload(token string, filePath string) (string, int64, error) {
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("读取文件失败: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", 0, fmt.Errorf("创建表单失败: %v", err)
	}
	_, err = part.Write(file)
	if err != nil {
		return "", 0, fmt.Errorf("写入文件失败: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return "", 0, fmt.Errorf("关闭表单失败: %v", err)
	}

	// 构建请求
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/upload?token=%s", c.BaseURL, token), body)
	if err != nil {
		return "", 0, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("解析响应失败: %v", err)
	}

	return result.Hash, result.Size, nil
}

// 从reader上传文件
func (c *ChunkServerClient) UploadFromReader(token string, fileName string, content io.Reader) (string, int64, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", 0, fmt.Errorf("创建表单失败: %v", err)
	}
	_, err = io.Copy(part, content)
	if err != nil {
		return "", 0, fmt.Errorf("写入文件失败: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return "", 0, fmt.Errorf("关闭表单失败: %v", err)
	}

	// 构建请求
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/upload?token=%s", c.BaseURL, token), body)
	if err != nil {
		return "", 0, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("解析响应失败: %v", err)
	}

	return result.Hash, result.Size, nil
}

// 下载文件
func (c *ChunkServerClient) Download(token string, hash string) ([]byte, error) {
	// 构建请求
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/download/%s?token=%s", c.BaseURL, hash, token), nil)
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
		respBody, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("下载失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 读取响应内容
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	return data, nil
}

// 下载文件到writer
func (c *ChunkServerClient) DownloadToWriter(token string, hash string, writer io.Writer) error {
	// 构建请求
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/download/%s?token=%s", c.BaseURL, hash, token), nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("下载失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 将响应内容写入writer
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("写入响应失败: %v", err)
	}

	return nil
}

// 保存分片
func (c *ChunkServerClient) SavePart(uploadID string, partNumber int, data []byte) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加分片
	part, err := writer.CreateFormFile("part", fmt.Sprintf("part%d", partNumber))
	if err != nil {
		return fmt.Errorf("创建表单失败: %v", err)
	}
	_, err = part.Write(data)
	if err != nil {
		return fmt.Errorf("写入分片失败: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("关闭表单失败: %v", err)
	}

	// 构建请求URL
	apiURL := fmt.Sprintf("%s/api/multipart/part?upload_id=%s&part_number=%d", c.BaseURL, url.QueryEscape(uploadID), partNumber)

	// 构建请求
	req, err := http.NewRequest("POST", apiURL, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("保存分片失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// 合并分片
func (c *ChunkServerClient) MergeParts(uploadID string, totalParts int, targetKey string) error {
	// 构建请求体
	reqBody := struct {
		UploadID   string `json:"upload_id"`
		TotalParts int    `json:"total_parts"`
		TargetKey  string `json:"target_key"`
	}{
		UploadID:   uploadID,
		TotalParts: totalParts,
		TargetKey:  targetKey,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求体失败: %v", err)
	}

	// 构建请求
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/multipart/merge", c.BaseURL), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("合并分片失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// 列出已上传分片
func (c *ChunkServerClient) ListUploadedParts(uploadID string) ([]int, error) {
	// 构建请求
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/multipart/parts/%s", c.BaseURL, url.QueryEscape(uploadID)), nil)
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
		respBody, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取分片列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var result struct {
		Parts []int `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return result.Parts, nil
}

// 删除上传临时文件
func (c *ChunkServerClient) RemoveUploadTemp(uploadID string) error {
	// 构建请求
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/multipart/%s", c.BaseURL, url.QueryEscape(uploadID)), nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 发送请求
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("删除临时文件失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// 生成上传令牌
func (c *ChunkServerClient) GenerateUploadToken(fileInfo map[string]interface{}, expireSeconds int) (string, error) {
	// 这个方法应该由主服务实现，这里只是示例
	// 实际上，令牌应该在主服务中生成并存储在Redis中，然后返回给客户端
	// 存储服务只负责验证令牌

	token := "upload_token_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	return token, nil
}

// 生成下载令牌
func (c *ChunkServerClient) GenerateDownloadToken(fileHash string, fileName string, expireSeconds int) (string, error) {
	// 这个方法应该由主服务实现，这里只是示例
	// 实际上，令牌应该在主服务中生成并存储在Redis中，然后返回给客户端
	// 存储服务只负责验证令牌

	token := "download_token_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	return token, nil
}

// ChunkClient 是与块存储服务通信的客户端
type ChunkClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewChunkClient 创建一个新的块存储服务客户端
func NewChunkClient(baseURL string) *ChunkClient {
	return &ChunkClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadFile 通过块存储服务上传文件
func (c *ChunkClient) UploadFile(filePath string, token string) (string, error) {
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
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("上传失败: %s", result.Message)
	}

	return result.Data.FileID, nil
}

// DownloadFile 通过块存储服务下载文件
func (c *ChunkClient) DownloadFile(fileID string, token string, destPath string) error {
	params := url.Values{}
	params.Add("file_id", fileID)
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

// InitMultipartUpload 初始化分片上传
func (c *ChunkClient) InitMultipartUpload(filename string, token string) (string, error) {
	params := url.Values{}
	params.Add("filename", filename)
	params.Add("token", token)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/multipart/init?%s", c.BaseURL, params.Encode()), nil)
	if err != nil {
		return "", err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("初始化分片上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			UploadID string `json:"upload_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("初始化分片上传失败: %s", result.Message)
	}

	return result.Data.UploadID, nil
}

// UploadPart 上传分片
func (c *ChunkClient) UploadPart(uploadID string, partNumber int, data []byte, token string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加token字段
	if err := writer.WriteField("token", token); err != nil {
		return "", err
	}

	// 添加uploadID字段
	if err := writer.WriteField("upload_id", uploadID); err != nil {
		return "", err
	}

	// 添加partNumber字段
	if err := writer.WriteField("part_number", strconv.Itoa(partNumber)); err != nil {
		return "", err
	}

	// 添加文件分片
	part, err := writer.CreateFormFile("part", fmt.Sprintf("part_%d", partNumber))
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/multipart/upload", c.BaseURL), body)
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
		return "", fmt.Errorf("上传分片失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ETag string `json:"etag"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("上传分片失败: %s", result.Message)
	}

	return result.Data.ETag, nil
}

// CompleteMultipartUpload 完成分片上传
func (c *ChunkClient) CompleteMultipartUpload(uploadID string, parts []PartInfo, token string) (string, error) {
	partsJSON, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加token字段
	if err := writer.WriteField("token", token); err != nil {
		return "", err
	}

	// 添加uploadID字段
	if err := writer.WriteField("upload_id", uploadID); err != nil {
		return "", err
	}

	// 添加parts字段
	if err := writer.WriteField("parts", string(partsJSON)); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/multipart/complete", c.BaseURL), body)
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
		return "", fmt.Errorf("完成分片上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FileID string `json:"file_id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("完成分片上传失败: %s", result.Message)
	}

	return result.Data.FileID, nil
}
