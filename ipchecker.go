package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type IPChecker struct {
	client   *http.Client
	services []string
	// 优先使用的服务（最可靠）
	primaryService string
}

func NewIPChecker() *IPChecker {
	return &IPChecker{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		// 优先使用最可靠的服务
		primaryService: "https://api.ipify.org",
		// 备用服务列表
		services: []string{
			"https://api.ipify.org",
			"https://ifconfig.me/ip",
			"https://icanhazip.com",
			"https://api.ip.sb/ip",
		},
	}
}

// GetPublicIP 获取公网IP，优先使用主服务，失败时尝试备用服务
func (ic *IPChecker) GetPublicIP() (string, error) {
	// 优先使用主服务
	ip, err := ic.getIPFromService(ic.primaryService)
	if err == nil && ip != "" && isValidIPv4(ip) {
		return ip, nil
	}

	// 主服务失败，尝试备用服务
	var lastErr error
	for _, service := range ic.services {
		if service == ic.primaryService {
			continue // 跳过已尝试的主服务
		}
		ip, err := ic.getIPFromService(service)
		if err == nil && ip != "" && isValidIPv4(ip) {
			return ip, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("所有IP检测服务均失败，最后错误: %v", lastErr)
}

// GetPublicIPWithService 获取公网IP并返回使用的服务名称
func (ic *IPChecker) GetPublicIPWithService() (string, string, error) {
	// 优先使用主服务
	ip, err := ic.getIPFromService(ic.primaryService)
	if err == nil && ip != "" && isValidIPv4(ip) {
		return ip, ic.primaryService, nil
	}

	// 主服务失败，尝试备用服务
	var lastErr error
	for _, service := range ic.services {
		if service == ic.primaryService {
			continue
		}
		ip, err := ic.getIPFromService(service)
		if err == nil && ip != "" && isValidIPv4(ip) {
			return ip, service, nil
		}
		lastErr = err
	}

	return "", "", fmt.Errorf("所有IP检测服务均失败，最后错误: %v", lastErr)
}

// isValidIPv4 验证是否为有效的IPv4地址
func isValidIPv4(ip string) bool {
	ip = strings.TrimSpace(ip)
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	// 确保是IPv4，不是IPv6
	return parsedIP.To4() != nil
}

func (ic *IPChecker) getIPFromService(url string) (string, error) {
	resp, err := ic.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("服务返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	if ip == "" {
		return "", fmt.Errorf("返回内容为空")
	}

	// 验证IP格式
	if !isValidIPv4(ip) {
		return "", fmt.Errorf("无效的IP地址格式: %s", ip)
	}

	return ip, nil
}
