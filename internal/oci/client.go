// Package oci 封装 Oracle Cloud Infrastructure 的 API 调用。
// 核心设计：每个账号、每个区域持有一组独立的客户端，且客户端绑定该账号专属的
// HTTP 代理，从而实现出口 IP 隔离，避免多账号共用同一 IP 触发风控。
package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/usageapi"

	"github.com/yourname/oci-manager/internal/store"
)

// Credentials 是调用 OCI API 所需的完整凭证。
type Credentials struct {
	TenancyOCID   string
	UserOCID      string
	Fingerprint   string
	PrivateKeyPEM string
	Passphrase    string
}

// ClientSet 是某个账号在某个区域下的一组服务客户端。
//
// Demo 不为 nil 时，这是一组「演示客户端」：所有 SDK 客户端都是 nil，
// oci 包的每个导出函数都会先短路到内存模拟数据。这样即便漏改某个函数，
// 也只会因 nil 指针报错，绝不会向 OCI 发出真实网络请求（fail-safe）。
type ClientSet struct {
	Region       string
	Compute      *core.ComputeClient
	Identity     *identity.IdentityClient
	Usage        *usageapi.UsageapiClient
	Network      *core.VirtualNetworkClient
	BlockStorage *core.BlockstorageClient
	Demo         *DemoState
}

// IsDemo 判断是否为演示客户端。
func (cs *ClientSet) IsDemo() bool { return cs != nil && cs.Demo != nil }

// newClientSet 构建一组客户端。proxyURL 为空表示直连。
func newClientSet(creds Credentials, region, proxyURL string) (*ClientSet, error) {
	var passphrase *string
	if creds.Passphrase != "" {
		p := creds.Passphrase
		passphrase = &p
	}

	// 凭证完全来自内存，不读取 ~/.oci/config，因此天然支持多账号并存。
	provider := common.NewRawConfigurationProvider(
		creds.TenancyOCID,
		creds.UserOCID,
		region,
		creds.Fingerprint,
		creds.PrivateKeyPEM,
		passphrase,
	)

	httpClient, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	compute, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Compute 客户端失败: %w", err)
	}
	compute.HTTPClient = httpClient

	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Identity 客户端失败: %w", err)
	}
	identityClient.HTTPClient = httpClient

	usageClient, err := usageapi.NewUsageapiClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建 Usage 客户端失败: %w", err)
	}
	usageClient.HTTPClient = httpClient

	networkClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建网络客户端失败: %w", err)
	}
	networkClient.HTTPClient = httpClient

	blockClient, err := core.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("创建块存储客户端失败: %w", err)
	}
	blockClient.HTTPClient = httpClient

	return &ClientSet{
		Region:       region,
		Compute:      &compute,
		Identity:     &identityClient,
		Usage:        &usageClient,
		Network:      &networkClient,
		BlockStorage: &blockClient,
	}, nil
}

// newHTTPClient 构造绑定指定代理的 http.Client。
// 支持 http、https 与 socks5 三种代理协议，形如：
//
//	http://user:pass@1.2.3.4:8080
//	socks5://user:pass@1.2.3.4:1080
func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		// 不读取 HTTP_PROXY/HTTPS_PROXY 环境变量：容器里若被注入恶意全局代理，
		// 会把所有 OCI 请求静默导向第三方。代理必须由账号显式配置。
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("代理地址格式错误: %w", err)
		}
		switch parsed.Scheme {
		case "http", "https", "socks5":
		default:
			return nil, fmt.Errorf("不支持的代理协议 %q，仅支持 http/https/socks5", parsed.Scheme)
		}
		// 阻止把代理指向云厂商元数据服务 / 链路本地地址，避免容器被当内网探测跳板。
		// 本机(127.0.0.1)与 RFC1918 地址属于「本机多代理」的合法用法，仍允许。
		host := strings.ToLower(parsed.Hostname())
		if ip := net.ParseIP(host); ip != nil && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
			return nil, fmt.Errorf("代理不能指向链路本地地址 %s（云元数据服务）", host)
		}
		if strings.Contains(host, "metadata") && strings.Contains(host, "internal") {
			return nil, fmt.Errorf("代理不能指向实例元数据服务 %s", host)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}

	return &http.Client{Transport: transport, Timeout: 90 * time.Second}, nil
}

// Pool 缓存账号客户端，避免每次请求都重新解析私钥与建立连接。
// 凭证或代理发生变化时会自动重建客户端。
type Pool struct {
	mu    sync.Mutex
	store *store.Store
	cache map[string]*poolEntry
	// demo 是全进程唯一的一份内存模拟状态，改名、换 IP 等操作会立刻反映在后续读取上。
	demo *DemoState
}

type poolEntry struct {
	clients     *ClientSet
	fingerprint string
}

// NewPool 创建客户端池。
func NewPool(st *store.Store) *Pool {
	return &Pool{store: st, cache: make(map[string]*poolEntry), demo: newDemoState()}
}

// DemoClientSet 返回演示客户端（不持有任何 SDK 客户端）。
func (p *Pool) DemoClientSet(region string) *ClientSet {
	if region == "" {
		region = "ap-tokyo-1"
	}
	return &ClientSet{Region: region, Demo: p.demo}
}

// IAMClientSet 用临时导入的高权限凭证构建只用于 IAM 的客户端。
// 演示账号直接返回内存模拟客户端，不需要任何凭证。
func (p *Pool) IAMClientSet(accountID string, creds Credentials, homeRegion string) (*ClientSet, error) {
	account, err := p.store.GetAccount(accountID)
	if err != nil {
		return nil, err
	}
	if account.IsDemo {
		return p.DemoClientSet(homeRegion), nil
	}
	if homeRegion == "" {
		return nil, fmt.Errorf("未配置 IAM home region，请在「设置 → IAM」中填写")
	}
	return newClientSet(creds, homeRegion, account.ProxyURL)
}

// For 返回指定账号在指定区域下的客户端。region 为空时使用账号的默认区域。
func (p *Pool) For(accountID, region string) (*ClientSet, error) {
	account, err := p.store.GetAccount(accountID)
	if err != nil {
		return nil, err
	}
	if region == "" {
		region = account.Region
	}
	if account.IsDemo {
		return p.DemoClientSet(region), nil
	}
	secret, err := p.store.AccountSecret(accountID)
	if err != nil {
		return nil, err
	}

	creds := Credentials{
		TenancyOCID:   account.TenancyOCID,
		UserOCID:      account.UserOCID,
		Fingerprint:   account.Fingerprint,
		PrivateKeyPEM: secret.PrivateKeyPEM,
		Passphrase:    secret.Passphrase,
	}
	fingerprint := credentialsFingerprint(creds, region, account.ProxyURL)

	key := accountID + "|" + region
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.cache[key]; ok && entry.fingerprint == fingerprint {
		return entry.clients, nil
	}

	clients, err := newClientSet(creds, region, account.ProxyURL)
	if err != nil {
		return nil, err
	}
	p.cache[key] = &poolEntry{clients: clients, fingerprint: fingerprint}
	return clients, nil
}

// Invalidate 丢弃某个账号的全部缓存客户端，修改账号信息后调用。
func (p *Pool) Invalidate(accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.cache {
		if len(key) >= len(accountID) && key[:len(accountID)] == accountID {
			delete(p.cache, key)
		}
	}
}

// credentialsFingerprint 计算凭证摘要，用于判断客户端是否需要重建。
// 摘要不含私钥明文以外的敏感信息泄露风险，仅用于内存比较。
func credentialsFingerprint(creds Credentials, region, proxyURL string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s", creds.TenancyOCID, creds.UserOCID,
		creds.Fingerprint, creds.PrivateKeyPEM, region, proxyURL)
	return hex.EncodeToString(h.Sum(nil))
}
