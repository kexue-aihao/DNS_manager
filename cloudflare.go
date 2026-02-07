package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CloudflareClient struct {
	apiToken string
	client   *http.Client
	baseURL  string
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type DNSRecordResponse struct {
	Result []DNSRecord `json:"result"`
	Success bool       `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type DNSRecordUpdateRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

type DNSRecordCreateRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

func NewCloudflareClient(apiToken string) (*CloudflareClient, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("API Token 不能为空")
	}

	return &CloudflareClient{
		apiToken: apiToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.cloudflare.com/client/v4",
	}, nil
}

func (c *CloudflareClient) makeRequest(method, endpoint string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + endpoint
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *CloudflareClient) ListDNSRecords(zoneID, recordName string) ([]DNSRecord, error) {
	endpoint := fmt.Sprintf("/zones/%s/dns_records?name=%s", zoneID, recordName)
	resp, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result DNSRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		var errorMsg string
		for _, e := range result.Errors {
			errorMsg += fmt.Sprintf("Code %d: %s; ", e.Code, e.Message)
		}
		return nil, fmt.Errorf("API 错误: %s", errorMsg)
	}

	return result.Result, nil
}

func (c *CloudflareClient) UpdateDNSRecord(zoneID, recordName, recordType, content string) error {
	// 首先查找现有的记录
	records, err := c.ListDNSRecords(zoneID, recordName)
	if err != nil {
		return fmt.Errorf("查找DNS记录失败: %v", err)
	}

	if len(records) == 0 {
		return fmt.Errorf("未找到匹配的DNS记录: %s", recordName)
	}

	// 查找匹配类型的记录
	var targetRecord *DNSRecord
	for i := range records {
		if records[i].Type == recordType {
			targetRecord = &records[i]
			break
		}
	}

	if targetRecord == nil {
		return fmt.Errorf("未找到类型为 %s 的DNS记录", recordType)
	}

	// 如果内容相同，跳过更新
	if targetRecord.Content == content {
		return nil
	}

	// 更新记录（使用乐观锁：先读取再更新）
	endpoint := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, targetRecord.ID)
	
	updateReq := DNSRecordUpdateRequest{
		Type:    recordType,
		Name:    recordName,
		Content: content,
		TTL:     targetRecord.TTL,
	}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	resp, err := c.makeRequest("PUT", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API 返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		Result  struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"result"`
		Errors   []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		var errorMsg string
		for _, e := range result.Errors {
			errorMsg += fmt.Sprintf("Code %d: %s; ", e.Code, e.Message)
		}
		return fmt.Errorf("API 错误: %s", errorMsg)
	}

	// 验证更新后的值是否正确
	if result.Result.Content != content {
		return fmt.Errorf("DNS记录更新后内容不匹配: 期望 %s，实际 %s", content, result.Result.Content)
	}

	return nil
}

// GetCurrentDNSRecord 获取当前DNS记录的值（返回第一个匹配的记录）
func (c *CloudflareClient) GetCurrentDNSRecord(zoneID, recordName, recordType string) (string, error) {
	records, err := c.ListDNSRecords(zoneID, recordName)
	if err != nil {
		return "", fmt.Errorf("查找DNS记录失败: %v", err)
	}

	for _, record := range records {
		if record.Type == recordType {
			return record.Content, nil
		}
	}

	return "", fmt.Errorf("未找到类型为 %s 的DNS记录", recordType)
}

// GetAllDNSRecords 获取所有匹配的DNS记录
func (c *CloudflareClient) GetAllDNSRecords(zoneID, recordName, recordType string) ([]DNSRecord, error) {
	records, err := c.ListDNSRecords(zoneID, recordName)
	if err != nil {
		return nil, fmt.Errorf("查找DNS记录失败: %v", err)
	}

	var matchedRecords []DNSRecord
	for _, record := range records {
		if record.Type == recordType {
			matchedRecords = append(matchedRecords, record)
		}
	}

	return matchedRecords, nil
}

// FindOrCreateDNSRecord 查找或创建DNS记录（支持多机器场景）
// 如果找到指向指定IP的记录，返回该记录；否则创建新记录
func (c *CloudflareClient) FindOrCreateDNSRecord(zoneID, recordName, recordType, content string, ttl int) (*DNSRecord, error) {
	// 获取所有匹配的记录
	records, err := c.GetAllDNSRecords(zoneID, recordName, recordType)
	if err != nil {
		// 如果没有找到记录，创建新记录
		return c.CreateDNSRecord(zoneID, recordName, recordType, content, ttl)
	}

	// 查找是否已有指向本机IP的记录
	for _, record := range records {
		if record.Content == content {
			// 找到指向本机IP的记录，返回它
			return &record, nil
		}
	}

	// 没有找到指向本机IP的记录，创建新记录
	return c.CreateDNSRecord(zoneID, recordName, recordType, content, ttl)
}

// CreateDNSRecord 创建新的DNS记录
func (c *CloudflareClient) CreateDNSRecord(zoneID, recordName, recordType, content string, ttl int) (*DNSRecord, error) {
	endpoint := fmt.Sprintf("/zones/%s/dns_records", zoneID)
	
	createReq := DNSRecordCreateRequest{
		Type:    recordType,
		Name:    recordName,
		Content: content,
		TTL:     ttl,
	}

	jsonData, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	resp, err := c.makeRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 (状态码: %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool      `json:"success"`
		Result  DNSRecord `json:"result"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		var errorMsg string
		for _, e := range result.Errors {
			errorMsg += fmt.Sprintf("Code %d: %s; ", e.Code, e.Message)
		}
		return nil, fmt.Errorf("API 错误: %s", errorMsg)
	}

	return &result.Result, nil
}

// UpdateOrCreateDNSRecord 更新或创建DNS记录（支持多机器场景）
// 优先查找指向本机IP的记录，如果不存在则创建新记录
// oldIP: 旧的IP地址，如果提供，会尝试更新指向旧IP的记录
func (c *CloudflareClient) UpdateOrCreateDNSRecord(zoneID, recordName, recordType, content string, ttl int, oldIP string) error {
	// 获取所有匹配的记录
	records, err := c.GetAllDNSRecords(zoneID, recordName, recordType)
	if err != nil || len(records) == 0 {
		// 如果没有找到任何记录，创建新记录
		_, err := c.CreateDNSRecord(zoneID, recordName, recordType, content, ttl)
		return err
	}

	// 查找是否已有指向本机IP的记录
	for _, record := range records {
		if record.Content == content {
			// 找到指向本机IP的记录，内容已匹配，无需更新
			return nil
		}
	}

	// 如果没有找到指向本机IP的记录，但有旧IP，尝试更新指向旧IP的记录
	if oldIP != "" {
		for _, record := range records {
			if record.Content == oldIP {
				// 找到指向旧IP的记录，更新它
				return c.UpdateDNSRecord(zoneID, recordName, recordType, content)
			}
		}
	}

	// 没有找到指向本机IP或旧IP的记录，创建新记录（支持多机器）
	_, err = c.CreateDNSRecord(zoneID, recordName, recordType, content, ttl)
	return err
}
